package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
	"github.com/zfouts/corescope-grafana-datasource/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

const (
	// MaxQueriesPerRequest bounds one QueryData call. A Viewer can POST any
	// number of queries to /api/ds/query; past this the whole call is refused.
	MaxQueriesPerRequest = 50
	// QueryConcurrency bounds upstream requests in flight per QueryData call.
	QueryConcurrency = 4
)

// Datasource is one configured CoreScope datasource instance.
type Datasource struct {
	api           corescope.API
	close         func()
	settingsErr   error // fixed-text validation error; reported by CheckHealth and every query
	tlsSkipVerify bool
	meshOnly      bool             // see MeshOnly below
	now           func() time.Time // nil -> time.Now; tests set it
}

// NewDatasource builds the instance. Invalid settings do not fail the
// factory: the instance keeps the error so the config page's "Save & test"
// shows it, and every query answers with it.
func NewDatasource(_ context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	settings, err := models.LoadPluginSettings(s)
	if err != nil {
		return &Datasource{settingsErr: errors.New("could not parse datasource settings")}, nil
	}
	c, err := corescope.NewClient(settings)
	if err != nil {
		return &Datasource{settingsErr: err}, nil
	}
	return &Datasource{api: c, close: c.Close, tlsSkipVerify: settings.TLSSkipVerify, meshOnly: settings.MeshOnly}, nil
}

// newWithAPI is the test constructor.
func newWithAPI(api corescope.API) *Datasource { return &Datasource{api: api} }

// Dispose releases the instance's idle connections.
func (d *Datasource) Dispose() {
	if d.close != nil {
		d.close()
	}
}

// QueryData answers each query independently, at most QueryConcurrency at a time.
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	resp := backend.NewQueryDataResponse()
	if len(req.Queries) > MaxQueriesPerRequest {
		for _, q := range req.Queries {
			resp.Responses[q.RefID] = backend.ErrDataResponse(backend.StatusBadRequest,
				fmt.Sprintf("too many queries in one request (max %d)", MaxQueriesPerRequest))
		}
		return resp, nil
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, QueryConcurrency)
	)
	for _, q := range req.Queries {
		wg.Add(1)
		sem <- struct{}{}
		go func(q backend.DataQuery) {
			defer wg.Done()
			defer func() { <-sem }()
			r := d.safeQuery(ctx, q)
			mu.Lock()
			resp.Responses[q.RefID] = r
			mu.Unlock()
		}(q)
	}
	wg.Wait()
	return resp, nil
}

// safeQuery turns a panic in one query into that query's error.
func (d *Datasource) safeQuery(ctx context.Context, q backend.DataQuery) (r backend.DataResponse) {
	defer func() {
		if p := recover(); p != nil {
			backend.Logger.Error("corescope: query panicked", "queryType", queryTypeOf(q.JSON), "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			r = backend.ErrDataResponse(backend.StatusInternal, "internal error")
		}
	}()
	return d.query(ctx, q)
}

func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	if d.settingsErr != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "datasource settings: "+d.settingsErr.Error())
	}
	qm, err := ParseQuery(q.JSON)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, err.Error())
	}
	if d.meshOnly && serverQueryTypes[qm.QueryType] {
		return backend.ErrDataResponse(backend.StatusForbidden, "query type not available on this datasource (mesh data only)")
	}
	frame, err := d.run(ctx, qm, q.TimeRange)
	if err != nil {
		return errResponse(ctx, err)
	}
	if d.meshOnly && qm.QueryType == QueryStats {
		frame = dropFields(frame, serverStatsFields)
	}
	frame.RefID = q.RefID
	return backend.DataResponse{Frames: data.Frames{frame}}
}

func (d *Datasource) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

// run maps one validated query to its upstream call and frame builder.
func (d *Datasource) run(ctx context.Context, qm QueryModel, tr backend.TimeRange) (*data.Frame, error) {
	from, to := tr.From.UTC(), tr.To.UTC()
	win := corescope.Window{From: from, To: to, Region: qm.Region}
	switch qm.QueryType {
	case QueryStats:
		return frameOf(d.api.Stats(ctx))(StatsFrame)
	case QueryHealth:
		return frameOf(d.api.Health(ctx))(HealthFrame)
	case QueryEndpoints:
		return frameOf(d.api.Perf(ctx))(EndpointsFrame)
	case QueryMqtt:
		return frameOf(d.api.MqttStatus(ctx))(MqttFrame)
	case QueryPacketsPerHour:
		return frameOf(d.api.RF(ctx, win))(PacketsPerHourFrame)
	case QueryPayloadTypes:
		return frameOf(d.api.RF(ctx, win))(PayloadTypesFrame)
	case QueryRetransmissions:
		return frameOf(d.api.Retransmissions(ctx, win))(RetransmissionsFrame)
	case QueryObserverMetrics:
		res := qm.Resolution
		if res == "" {
			res = ObserverResolution(from, to)
		}
		return frameOf(d.api.ObserverMetrics(ctx, qm.ObserverID, from, to, res))(ObserverMetricsFrame)
	case QueryNodeActivity:
		a, err := d.api.NodeAnalytics(ctx, qm.NodePubkey, DaysForRange(d.clock(), from))
		if err != nil {
			return nil, err
		}
		return NodeActivityFrame(a, from, to), nil
	case QueryNodeBattery:
		b, err := d.api.NodeBattery(ctx, qm.NodePubkey, DaysForRange(d.clock(), from))
		if err != nil {
			return nil, err
		}
		return NodeBatteryFrame(b, from, to), nil
	case QueryNodes:
		return frameOf(d.api.Nodes(ctx, corescope.NodeQuery{Limit: qm.Limit, Role: qm.Role, Region: qm.Region, Search: qm.Search}))(NodesFrame)
	case QueryObservers:
		return frameOf(d.api.Observers(ctx))(ObserversFrame)
	}
	// ParseQuery already rejected anything else.
	return nil, &QueryError{Field: "queryType"}
}

// MeshOnly. A Viewer can POST any queryType to /api/ds/query, so hiding
// panels does not keep server internals off a public dashboard. With
// jsonData.meshOnly the backend refuses these query types outright and strips
// these fields from the stats frame.
var serverQueryTypes = map[QueryType]bool{QueryHealth: true, QueryEndpoints: true, QueryMqtt: true}

var serverStatsFields = map[string]bool{
	"storeDataMB": true, "processRSSMB": true, "goHeapInuseMB": true, "goSysMB": true,
	"version": true, "commit": true,
}

// dropFields returns frame without the named fields (same name, refID, meta).
func dropFields(f *data.Frame, drop map[string]bool) *data.Frame {
	kept := f.Fields[:0:0]
	for _, fld := range f.Fields {
		if !drop[fld.Name] {
			kept = append(kept, fld)
		}
	}
	f.Fields = kept
	return f
}

// frameOf adapts an (value, error) API result to a frame builder.
func frameOf[T any](v *T, err error) func(func(*T) *data.Frame) (*data.Frame, error) {
	return func(build func(*T) *data.Frame) (*data.Frame, error) {
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, &corescope.Error{Status: http.StatusBadGateway, Message: "empty response"}
		}
		return build(v), nil
	}
}

// errResponse maps a client error to a query error. Messages are the
// client's fixed or sanitized text; the upstream path is not shown.
func errResponse(ctx context.Context, err error) backend.DataResponse {
	var ce *corescope.Error
	var qe *QueryError
	switch {
	case errors.As(err, &qe):
		return backend.ErrDataResponse(backend.StatusBadRequest, qe.Error())
	case errors.As(err, &ce):
		return backend.ErrDataResponseWithSource(backend.Status(ce.Status),
			backend.ErrorSourceFromHTTPStatus(ce.Status), "CoreScope: "+ce.Message)
	case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return backend.ErrDataResponse(backend.StatusTimeout, "request cancelled or timed out")
	}
	return backend.ErrDataResponse(backend.StatusInternal, "internal error")
}

// queryTypeOf reads only the queryType for logging; never the whole query.
func queryTypeOf(raw json.RawMessage) string {
	var q struct {
		QueryType string `json:"queryType"`
	}
	_ = json.Unmarshal(raw, &q)
	return SafeString(q.QueryType)
}

// CheckHealth backs "Save & test": settings, readiness, then version.
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	fail := func(msg string) (*backend.CheckHealthResult, error) {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: msg}, nil
	}
	if d.settingsErr != nil {
		return fail("Settings: " + d.settingsErr.Error())
	}
	hz, err := d.api.Healthz(ctx)
	if err != nil {
		return fail(healthErr(err))
	}
	if !hz.Ready {
		return fail("CoreScope is reachable but still starting up; try again in a minute")
	}
	st, err := d.api.Stats(ctx)
	if err != nil {
		return fail(healthErr(err))
	}
	msg := fmt.Sprintf("Connected to CoreScope %s: %d nodes, %d observers",
		SafeString(st.Version), st.TotalNodes, st.TotalObservers)
	if d.tlsSkipVerify {
		msg += ". Warning: TLS certificate verification is off"
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: msg}, nil
}

func healthErr(err error) string {
	var ce *corescope.Error
	if errors.As(err, &ce) {
		return fmt.Sprintf("CoreScope: %s (HTTP %d)", ce.Message, ce.Status)
	}
	return "CoreScope request failed or timed out"
}

// CallResource serves template-variable lookups. Only GET "observers" and
// GET "regions" exist; req.Path is matched, never forwarded.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	reply := func(status int, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return sender.Send(&backend.CallResourceResponse{
			Status:  status,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b,
		})
	}
	type errBody struct {
		Error string `json:"error"`
	}
	if req.Method != http.MethodGet {
		return reply(http.StatusNotFound, errBody{"not found"})
	}
	if d.settingsErr != nil {
		return reply(http.StatusBadRequest, errBody{"datasource settings: " + d.settingsErr.Error()})
	}

	var (
		opts []MetricFindOption
		err  error
	)
	switch strings.Trim(req.Path, "/") {
	case "observers":
		opts, err = d.observerOptions(ctx)
	case "regions":
		opts, err = d.regionOptions(ctx)
	default:
		return reply(http.StatusNotFound, errBody{"not found"})
	}
	if err != nil {
		var ce *corescope.Error
		if errors.As(err, &ce) {
			return reply(http.StatusBadGateway, errBody{"CoreScope: " + ce.Message})
		}
		return reply(http.StatusBadGateway, errBody{"CoreScope request failed"})
	}
	return reply(http.StatusOK, opts)
}

// MetricFindOption mirrors src/types.ts MetricFindOption.
type MetricFindOption struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

func (d *Datasource) observerOptions(ctx context.Context) ([]MetricFindOption, error) {
	l, err := d.api.Observers(ctx)
	if err != nil {
		return nil, err
	}
	opts := make([]MetricFindOption, 0, len(l.Observers))
	for _, o := range l.Observers {
		id := SafeString(o.ID)
		text := SafeStringPtr(o.Name)
		if text == "" {
			text = id
		}
		opts = append(opts, MetricFindOption{Text: text, Value: id})
	}
	sort.SliceStable(opts, func(i, j int) bool { return opts[i].Text < opts[j].Text })
	return opts, nil
}

func (d *Datasource) regionOptions(ctx context.Context) ([]MetricFindOption, error) {
	r, err := d.api.Regions(ctx)
	if err != nil {
		return nil, err
	}
	opts := make([]MetricFindOption, 0, len(r))
	for code, label := range r {
		code = SafeString(code)
		label = SafeString(label)
		if label == "" {
			label = code
		}
		opts = append(opts, MetricFindOption{Text: label, Value: code})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
	return opts, nil
}
