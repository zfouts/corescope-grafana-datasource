package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

// fakeAPI serves the recorded fixtures and records what it was asked.
type fakeAPI struct {
	t        *testing.T
	err      error // returned by every method when set
	panicOn  string
	notReady bool

	mu       sync.Mutex
	calls    []string
	lastWin  corescope.Window
	lastRes  string
	lastDays int
	lastNode corescope.NodeQuery
	inFlight int32
	maxSeen  int32
}

func (f *fakeAPI) record(name string) error {
	n := atomic.AddInt32(&f.inFlight, 1)
	defer atomic.AddInt32(&f.inFlight, -1)
	for {
		m := atomic.LoadInt32(&f.maxSeen)
		if n <= m || atomic.CompareAndSwapInt32(&f.maxSeen, m, n) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond) // let concurrent queries overlap
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
	if f.panicOn == name {
		panic("boom")
	}
	return f.err
}

func fixture[T any](f *fakeAPI, name string) *T {
	f.t.Helper()
	b, err := os.ReadFile("../corescope/testdata/" + name + ".json")
	if err != nil {
		f.t.Fatal(err)
	}
	v := new(T)
	if err := json.Unmarshal(b, v); err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fakeAPI) Healthz(context.Context) (*corescope.Healthz, error) {
	if err := f.record("healthz"); err != nil {
		return nil, err
	}
	return &corescope.Healthz{Ready: !f.notReady}, nil
}
func (f *fakeAPI) Stats(context.Context) (*corescope.Stats, error) {
	if err := f.record("stats"); err != nil {
		return nil, err
	}
	return fixture[corescope.Stats](f, "stats"), nil
}
func (f *fakeAPI) Health(context.Context) (*corescope.Health, error) {
	if err := f.record("health"); err != nil {
		return nil, err
	}
	return fixture[corescope.Health](f, "health"), nil
}
func (f *fakeAPI) Perf(context.Context) (*corescope.Perf, error) {
	if err := f.record("perf"); err != nil {
		return nil, err
	}
	return fixture[corescope.Perf](f, "perf"), nil
}
func (f *fakeAPI) MqttStatus(context.Context) (*corescope.MqttStatus, error) {
	if err := f.record("mqtt"); err != nil {
		return nil, err
	}
	return fixture[corescope.MqttStatus](f, "mqtt_status_sources"), nil
}
func (f *fakeAPI) RF(_ context.Context, w corescope.Window) (*corescope.RFAnalytics, error) {
	f.mu.Lock()
	f.lastWin = w
	f.mu.Unlock()
	if err := f.record("rf"); err != nil {
		return nil, err
	}
	return fixture[corescope.RFAnalytics](f, "analytics_rf_signal"), nil
}
func (f *fakeAPI) Retransmissions(_ context.Context, w corescope.Window) (*corescope.Retransmissions, error) {
	f.mu.Lock()
	f.lastWin = w
	f.mu.Unlock()
	if err := f.record("retransmissions"); err != nil {
		return nil, err
	}
	return fixture[corescope.Retransmissions](f, "analytics_retransmissions"), nil
}
func (f *fakeAPI) ObserverMetrics(_ context.Context, id string, _, _ time.Time, res string) (*corescope.ObserverMetrics, error) {
	f.mu.Lock()
	f.lastRes = res
	f.mu.Unlock()
	if err := f.record("observerMetrics:" + id); err != nil {
		return nil, err
	}
	return fixture[corescope.ObserverMetrics](f, "observers_ID_metrics"), nil
}
func (f *fakeAPI) NodeAnalytics(_ context.Context, pk string, days int) (*corescope.NodeAnalytics, error) {
	f.mu.Lock()
	f.lastDays = days
	f.mu.Unlock()
	if err := f.record("nodeAnalytics:" + pk); err != nil {
		return nil, err
	}
	return fixture[corescope.NodeAnalytics](f, "nodes_PK_analytics"), nil
}
func (f *fakeAPI) NodeBattery(_ context.Context, pk string, days int) (*corescope.NodeBattery, error) {
	f.mu.Lock()
	f.lastDays = days
	f.mu.Unlock()
	if err := f.record("nodeBattery:" + pk); err != nil {
		return nil, err
	}
	return fixture[corescope.NodeBattery](f, "nodes_PK_battery"), nil
}
func (f *fakeAPI) Nodes(_ context.Context, q corescope.NodeQuery) (*corescope.NodeList, error) {
	f.mu.Lock()
	f.lastNode = q
	f.mu.Unlock()
	if err := f.record("nodes"); err != nil {
		return nil, err
	}
	return fixture[corescope.NodeList](f, "nodes"), nil
}
func (f *fakeAPI) Observers(context.Context) (*corescope.ObserverList, error) {
	if err := f.record("observers"); err != nil {
		return nil, err
	}
	return fixture[corescope.ObserverList](f, "observers"), nil
}
func (f *fakeAPI) Regions(context.Context) (corescope.Regions, error) {
	if err := f.record("regions"); err != nil {
		return nil, err
	}
	return *fixture[corescope.Regions](f, "config_regions"), nil
}

const testPubkey = "988396af0ff369c2178a8c2a303661bf26193257136ed4452b6eadd9547011e9"

var testRange = backend.TimeRange{
	From: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2026, 11, 24, 2, 0, 0, 0, time.UTC),
}

func dq(ref, js string) backend.DataQuery {
	return backend.DataQuery{RefID: ref, JSON: json.RawMessage(js), TimeRange: testRange}
}

func runQueries(t *testing.T, d *Datasource, qs ...backend.DataQuery) *backend.QueryDataResponse {
	t.Helper()
	resp, err := d.QueryData(context.Background(), &backend.QueryDataRequest{Queries: qs})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestQueryDataEveryType(t *testing.T) {
	cases := []struct {
		js        string
		frameName string
		minRows   int
	}{
		{`{"queryType":"stats"}`, "stats", 1},
		{`{"queryType":"health"}`, "health", 1},
		{`{"queryType":"endpoints"}`, "endpoints", 1},
		{`{"queryType":"mqtt"}`, "mqtt", 2},
		{`{"queryType":"packetsPerHour","region":"OAK"}`, "packetsPerHour", 2},
		{`{"queryType":"payloadTypes"}`, "payloadTypes", 2},
		{`{"queryType":"retransmissions"}`, "retransmissions", 1},
		{`{"queryType":"observerMetrics","observerId":"D5EF2ED77CEABCB2"}`, "Kennedy Repeater", 4},
		{`{"queryType":"nodeActivity","nodePubkey":"` + strings.ToUpper(testPubkey) + `"}`, "nodeActivity", 1},
		{`{"queryType":"nodeBattery","nodePubkey":"` + testPubkey + `"}`, "nodeBattery", 3},
		{`{"queryType":"nodes","role":"repeater","limit":9999}`, "nodes", 5},
		{`{"queryType":"observers"}`, "observers", 3},
	}
	for _, c := range cases {
		t.Run(c.frameName, func(t *testing.T) {
			d := newWithAPI(&fakeAPI{t: t})
			r := runQueries(t, d, dq("A", c.js)).Responses["A"]
			if r.Error != nil {
				t.Fatalf("error: %v", r.Error)
			}
			if len(r.Frames) != 1 {
				t.Fatalf("frames = %d, want 1", len(r.Frames))
			}
			f := r.Frames[0]
			if f.Name != c.frameName || f.RefID != "A" {
				t.Fatalf("frame name/ref = %q/%q", f.Name, f.RefID)
			}
			n, err := f.RowLen()
			if err != nil || n < c.minRows {
				t.Fatalf("rows = %d (err %v), want >= %d", n, err, c.minRows)
			}
		})
	}
}

func TestQueryDataPassesValidatedParams(t *testing.T) {
	f := &fakeAPI{t: t}
	d := newWithAPI(f)
	d.now = func() time.Time { return testRange.To }

	runQueries(t, d, dq("A", `{"queryType":"packetsPerHour","region":"OAK,SJC"}`))
	if f.lastWin.Region != "OAK,SJC" || !f.lastWin.From.Equal(testRange.From) || !f.lastWin.To.Equal(testRange.To) {
		t.Fatalf("window = %+v", f.lastWin)
	}
	runQueries(t, d, dq("A", `{"queryType":"retransmissions","region":"$__all"}`))
	if f.lastWin.Region != "" {
		t.Fatalf("$__all region = %q, want empty", f.lastWin.Region)
	}
	runQueries(t, d, dq("A", `{"queryType":"observerMetrics","observerId":"ABC"}`))
	if f.lastRes != "1h" { // 7-day range → auto 1h
		t.Fatalf("auto resolution = %q", f.lastRes)
	}
	runQueries(t, d, dq("A", `{"queryType":"observerMetrics","observerId":"ABC","resolution":"1d"}`))
	if f.lastRes != "1d" {
		t.Fatalf("explicit resolution = %q", f.lastRes)
	}
	runQueries(t, d, dq("A", `{"queryType":"nodeBattery","nodePubkey":"`+testPubkey+`"}`))
	if f.lastDays != 8 { // now - from = 7d 2h → ceil 8
		t.Fatalf("days = %d", f.lastDays)
	}
	// Past range: CoreScope counts days back from its own clock, so days
	// reaches now-8d even though the range spans only 2 days.
	now := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return now }
	past := backend.TimeRange{From: now.Add(-8 * 24 * time.Hour), To: now.Add(-6 * 24 * time.Hour)}
	pq := backend.DataQuery{RefID: "A", TimeRange: past,
		JSON: json.RawMessage(`{"queryType":"nodeBattery","nodePubkey":"` + testPubkey + `"}`)}
	resp := runQueries(t, d, pq)
	if f.lastDays != 8 {
		t.Fatalf("past-range days = %d, want 8", f.lastDays)
	}
	r := resp.Responses["A"]
	if r.Error != nil || len(r.Frames) != 1 || r.Frames[0].Rows() != 2 {
		t.Fatalf("past range rows: err=%v frames=%v", r.Error, r.Frames)
	}
	runQueries(t, d, dq("A", `{"queryType":"nodes","role":"room","limit":9999,"region":"OAK","search":"ab"}`))
	if f.lastNode != (corescope.NodeQuery{Limit: 500, Role: "room", Region: "OAK", Search: "ab"}) {
		t.Fatalf("node query = %+v", f.lastNode)
	}
}

func TestQueryDataRejectsBadInputWithoutCallingUpstream(t *testing.T) {
	bad := []string{
		`{"queryType":"backup"}`,
		`{"queryType":""}`,
		`not json`,
		`{"queryType":"observerMetrics","observerId":".."}`,
		`{"queryType":"observerMetrics","observerId":"a/b"}`,
		`{"queryType":"observerMetrics","observerId":"x?y=1"}`,
		`{"queryType":"nodeBattery","nodePubkey":"../../api/backup"}`,
		`{"queryType":"nodes","role":"admin"}`,
		`{"queryType":"packetsPerHour","region":"OAK&x=1"}`,
	}
	for _, js := range bad {
		f := &fakeAPI{t: t}
		r := runQueries(t, newWithAPI(f), dq("A", js)).Responses["A"]
		if r.Error == nil || r.Status != backend.StatusBadRequest {
			t.Errorf("%s: status %v err %v, want 400", js, r.Status, r.Error)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: upstream called: %v", js, f.calls)
		}
		if strings.Contains(r.Error.Error(), "backup") || strings.Contains(r.Error.Error(), "a/b") {
			t.Errorf("%s: error echoes the input: %v", js, r.Error)
		}
	}
}

func TestQueryDataUpstreamErrorMapping(t *testing.T) {
	f := &fakeAPI{t: t, err: &corescope.Error{Status: 503, Path: "/api/analytics/rf", Message: "Service Unavailable"}}
	r := runQueries(t, newWithAPI(f), dq("A", `{"queryType":"packetsPerHour"}`)).Responses["A"]
	if r.Status != backend.Status(503) || r.ErrorSource != backend.ErrorSourceDownstream {
		t.Fatalf("status %v source %v", r.Status, r.ErrorSource)
	}
	if got := r.Error.Error(); got != "CoreScope: Service Unavailable" {
		t.Fatalf("message %q", got)
	}

	f = &fakeAPI{t: t, err: context.DeadlineExceeded}
	r = runQueries(t, newWithAPI(f), dq("A", `{"queryType":"stats"}`)).Responses["A"]
	if r.Status != backend.StatusTimeout {
		t.Fatalf("deadline status %v", r.Status)
	}

	f = &fakeAPI{t: t, err: fmt.Errorf("secret detail")}
	r = runQueries(t, newWithAPI(f), dq("A", `{"queryType":"stats"}`)).Responses["A"]
	if r.Status != backend.StatusInternal || strings.Contains(r.Error.Error(), "secret") {
		t.Fatalf("unknown error leaked: %v %v", r.Status, r.Error)
	}
}

func TestQueryDataPanicIsolated(t *testing.T) {
	f := &fakeAPI{t: t, panicOn: "stats"}
	resp := runQueries(t, newWithAPI(f), dq("A", `{"queryType":"stats"}`), dq("B", `{"queryType":"health"}`))
	if a := resp.Responses["A"]; a.Status != backend.StatusInternal || a.Error.Error() != "internal error" {
		t.Fatalf("A = %v %v", a.Status, a.Error)
	}
	if b := resp.Responses["B"]; b.Error != nil || len(b.Frames) != 1 {
		t.Fatalf("B affected by A's panic: %v", b.Error)
	}
}

func TestQueryDataLimits(t *testing.T) {
	f := &fakeAPI{t: t}
	d := newWithAPI(f)

	qs := make([]backend.DataQuery, MaxQueriesPerRequest+1)
	for i := range qs {
		qs[i] = dq(fmt.Sprintf("Q%d", i), `{"queryType":"stats"}`)
	}
	resp := runQueries(t, d, qs...)
	if len(f.calls) != 0 {
		t.Fatalf("over-limit request reached upstream %d times", len(f.calls))
	}
	if r := resp.Responses["Q0"]; r.Status != backend.StatusBadRequest {
		t.Fatalf("over-limit status %v", r.Status)
	}

	resp = runQueries(t, d, qs[:20]...)
	if len(resp.Responses) != 20 || len(f.calls) != 20 {
		t.Fatalf("responses %d calls %d", len(resp.Responses), len(f.calls))
	}
	if m := atomic.LoadInt32(&f.maxSeen); m > QueryConcurrency || m < 2 {
		t.Fatalf("max concurrency seen %d, want 2..%d", m, QueryConcurrency)
	}
}

func TestSettingsErrorSurfaces(t *testing.T) {
	inst, err := NewDatasource(context.Background(), backend.DataSourceInstanceSettings{
		JSONData: json.RawMessage(`{"url":"https://user:pw@example.net"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	d := inst.(*Datasource)
	if d.settingsErr == nil {
		t.Fatal("userinfo URL accepted")
	}
	h, _ := d.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if h.Status != backend.HealthStatusError || strings.Contains(h.Message, "pw") {
		t.Fatalf("health = %v %q", h.Status, h.Message)
	}
	r := runQueries(t, d, dq("A", `{"queryType":"stats"}`)).Responses["A"]
	if r.Status != backend.StatusBadRequest || strings.Contains(r.Error.Error(), "pw") {
		t.Fatalf("query = %v %v", r.Status, r.Error)
	}

	inst, err = NewDatasource(context.Background(), backend.DataSourceInstanceSettings{
		JSONData:                json.RawMessage(`{"url":"https://example.net/","tlsSkipVerify":true}`),
		DecryptedSecureJSONData: map[string]string{"bearerToken": "tok"},
	})
	if err != nil || inst.(*Datasource).settingsErr != nil {
		t.Fatalf("valid settings rejected: %v %v", err, inst.(*Datasource).settingsErr)
	}
	inst.(*Datasource).Dispose()
}

func TestCheckHealth(t *testing.T) {
	ctx := context.Background()
	h, _ := newWithAPI(&fakeAPI{t: t}).CheckHealth(ctx, nil)
	if h.Status != backend.HealthStatusOk || !strings.Contains(h.Message, "200 nodes, 31 observers") {
		t.Fatalf("ok case: %v %q", h.Status, h.Message)
	}

	d := newWithAPI(&fakeAPI{t: t})
	d.tlsSkipVerify = true
	if h, _ := d.CheckHealth(ctx, nil); !strings.Contains(h.Message, "TLS certificate verification is off") {
		t.Fatalf("no TLS warning: %q", h.Message)
	}

	if h, _ := newWithAPI(&fakeAPI{t: t, notReady: true}).CheckHealth(ctx, nil); h.Status != backend.HealthStatusError || !strings.Contains(h.Message, "starting up") {
		t.Fatalf("not ready: %v %q", h.Status, h.Message)
	}

	f := &fakeAPI{t: t, err: &corescope.Error{Status: 502, Message: "upstream returned 200 text/html; the URL may not point at a CoreScope API"}}
	if h, _ := newWithAPI(f).CheckHealth(ctx, nil); h.Status != backend.HealthStatusError || !strings.Contains(h.Message, "text/html") || !strings.Contains(h.Message, "HTTP 502") {
		t.Fatalf("error case: %v %q", h.Status, h.Message)
	}
}

type captureSender struct{ resp *backend.CallResourceResponse }

func (c *captureSender) Send(r *backend.CallResourceResponse) error { c.resp = r; return nil }

func callResource(t *testing.T, d *Datasource, method, path string) (int, []MetricFindOption, string) {
	t.Helper()
	s := &captureSender{}
	if err := d.CallResource(context.Background(), &backend.CallResourceRequest{Method: method, Path: path}, s); err != nil {
		t.Fatal(err)
	}
	var opts []MetricFindOption
	if s.resp.Status == http.StatusOK {
		if err := json.Unmarshal(s.resp.Body, &opts); err != nil {
			t.Fatal(err)
		}
	}
	return s.resp.Status, opts, string(s.resp.Body)
}

func TestCallResource(t *testing.T) {
	f := &fakeAPI{t: t}
	d := newWithAPI(f)

	st, opts, _ := callResource(t, d, "GET", "observers")
	if st != 200 || len(opts) != 3 {
		t.Fatalf("observers: %d %v", st, opts)
	}
	for i := 1; i < len(opts); i++ {
		if opts[i-1].Text > opts[i].Text {
			t.Fatalf("observers not sorted: %v", opts)
		}
	}
	if opts[0].Value == "" || opts[0].Text == "" {
		t.Fatalf("empty option: %v", opts[0])
	}

	st, opts, _ = callResource(t, d, "GET", "/regions")
	if st != 200 || len(opts) != 4 || opts[0].Value != "MRY" || opts[3].Value != "SJC" {
		t.Fatalf("regions: %d %v", st, opts)
	}

	calls := len(f.calls)
	for _, c := range [][2]string{
		{"POST", "observers"},
		{"GET", "nodes"},
		{"GET", "../api/backup"},
		{"GET", "observers/../../api/backup"},
		{"GET", "api/stats"},
		{"GET", ""},
	} {
		if st, _, _ := callResource(t, d, c[0], c[1]); st != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", c[0], c[1], st)
		}
	}
	if len(f.calls) != calls {
		t.Fatalf("rejected resource paths reached upstream: %v", f.calls[calls:])
	}

	f.err = &corescope.Error{Status: 500, Message: "boom"}
	if st, _, body := callResource(t, d, "GET", "observers"); st != http.StatusBadGateway || !strings.Contains(body, "CoreScope: boom") {
		t.Fatalf("upstream error: %d %s", st, body)
	}
}

func TestMeshOnly(t *testing.T) {
	inst, err := NewDatasource(context.Background(), backend.DataSourceInstanceSettings{
		JSONData: json.RawMessage(`{"url":"https://example.net","meshOnly":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inst.(*Datasource).meshOnly {
		t.Fatal("meshOnly not read from jsonData")
	}

	f := &fakeAPI{t: t}
	d := newWithAPI(f)
	d.meshOnly = true
	for _, qt := range []string{"health", "endpoints", "mqtt"} {
		r := runQueries(t, d, dq("A", `{"queryType":"`+qt+`"}`)).Responses["A"]
		if r.Status != backend.StatusForbidden || r.Error == nil || len(r.Frames) != 0 {
			t.Fatalf("%s: status %v err %v frames %d, want 403 and no frame", qt, r.Status, r.Error, len(r.Frames))
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("refused query types still called upstream: %v", f.calls)
	}

	r := runQueries(t, d, dq("A", `{"queryType":"stats"}`)).Responses["A"]
	if r.Error != nil || len(r.Frames) != 1 {
		t.Fatalf("stats: %v", r.Error)
	}
	have := map[string]bool{}
	for _, fld := range r.Frames[0].Fields {
		have[fld.Name] = true
	}
	for name := range serverStatsFields {
		if have[name] {
			t.Fatalf("stats kept server field %q with meshOnly", name)
		}
	}
	for _, name := range []string{"totalPackets", "totalNodes", "totalObservers", "packetsLast24h", "repeaters"} {
		if !have[name] {
			t.Fatalf("stats lost mesh field %q", name)
		}
	}

	// Mesh query types still work.
	for _, js := range []string{`{"queryType":"packetsPerHour"}`, `{"queryType":"nodes"}`, `{"queryType":"observers"}`} {
		if r := runQueries(t, d, dq("A", js)).Responses["A"]; r.Error != nil {
			t.Fatalf("%s: %v", js, r.Error)
		}
	}

	// Off (the default), everything is served and stats is whole.
	d.meshOnly = false
	r = runQueries(t, d, dq("A", `{"queryType":"stats"}`)).Responses["A"]
	if r.Error != nil || len(r.Frames[0].Fields) != 19 {
		t.Fatalf("stats off: %v, %d fields, want 19", r.Error, len(r.Frames[0].Fields))
	}
	if r := runQueries(t, d, dq("A", `{"queryType":"health"}`)).Responses["A"]; r.Error != nil {
		t.Fatalf("health off: %v", r.Error)
	}
}
