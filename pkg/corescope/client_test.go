package corescope

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zfouts/corescope-grafana-datasource/pkg/models"
)

const (
	testPubkey     = "988396af0ff369c2178a8c2a303661bf26193257136ed4452b6eadd9547011e9"
	testObserverID = "D5EF2ED77CEABCB22082F8794D5F9D89F2257E70533863E924DEBE6EBDDA312A"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// recorder counts requests and keeps the last one.
type recorder struct {
	mu   sync.Mutex
	hits atomic.Int32
	last *http.Request
}

func (r *recorder) record(req *http.Request) {
	r.hits.Add(1)
	r.mu.Lock()
	r.last = req.Clone(context.Background())
	r.mu.Unlock()
}

func (r *recorder) lastReq() *http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func serveJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func newTestClient(t *testing.T, srvURL string, mutate func(*models.PluginSettings)) *Client {
	t.Helper()
	s := &models.PluginSettings{URL: srvURL, TimeoutSeconds: 5}
	if mutate != nil {
		mutate(s)
	}
	c, err := NewClient(s)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func wantError(t *testing.T, err error, status int, msg string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v (%T), want *Error", err, err)
	}
	if status != 0 && e.Status != status {
		t.Errorf("Status = %d, want %d (msg %q)", e.Status, status, e.Message)
	}
	if msg != "" && e.Message != msg {
		t.Errorf("Message = %q, want %q", e.Message, msg)
	}
	return e
}

// fixtureServer serves every route from testdata and records requests.
func fixtureServer(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	routes := map[string]string{
		"/api/stats":                     "stats.json",
		"/api/health":                    "health.json",
		"/api/perf":                      "perf.json",
		"/api/mqtt/status":               "mqtt_status_sources.json",
		"/api/analytics/rf":              "analytics_rf_signal.json",
		"/api/analytics/retransmissions": "analytics_retransmissions.json",
		"/api/observers/" + testObserverID + "/metrics": "observers_ID_metrics.json",
		"/api/nodes/" + testPubkey + "/analytics":       "nodes_PK_analytics.json",
		"/api/nodes/" + testPubkey + "/battery":         "nodes_PK_battery.json",
		"/api/nodes":                                    "nodes.json",
		"/api/observers":                                "observers.json",
		"/api/config/regions":                           "config_regions.json",
	}
	bodies := map[string][]byte{"/api/healthz": []byte(`{"ready":true}`)}
	for p, f := range routes {
		bodies[p] = fixture(t, f)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		if b, ok := bodies[r.URL.Path]; ok {
			serveJSON(w, 200, b)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(fixture(t, "spa_fallback.html"))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestClientDecodesFixtures(t *testing.T) {
	srv, rec := fixtureServer(t)
	c := newTestClient(t, srv.URL, nil)
	ctx := context.Background()
	from := time.Date(2026, 11, 23, 22, 0, 0, 0, time.FixedZone("x", 3600))
	to := from.Add(3 * time.Hour)

	t.Run("Healthz", func(t *testing.T) {
		h, err := c.Healthz(ctx)
		if err != nil || !h.Ready {
			t.Fatalf("%+v %v", h, err)
		}
	})
	t.Run("Stats", func(t *testing.T) {
		s, err := c.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if s.TotalPackets != 499 || s.TotalNodes != 200 || s.Counts.Repeaters != 168 || s.TotalTransmissions == nil || *s.TotalTransmissions != 499 || s.GoSysMB != 39.9 {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("Health", func(t *testing.T) {
		h, err := c.Health(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if h.Status != "ok" || h.Uptime != 6 || h.Memory.RSS != 39 || h.PacketStore.Packets != 499 || h.EventLoop.P95Ms != 0.2 {
			t.Fatalf("%+v", h)
		}
	})
	t.Run("Perf", func(t *testing.T) {
		p, err := c.Perf(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if p.TotalRequests != 6 || p.Endpoints["/api/healthz"].Count != 2 || p.Endpoints["/api/observers"].AvgMs != 1.2 {
			t.Fatalf("%+v", p)
		}
	})
	t.Run("MqttStatus", func(t *testing.T) {
		m, err := c.MqttStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Sources) != 2 || m.Sources[0].Name != "primary" || !m.Sources[0].Connected || m.Sources[1].LastError != "dial tcp: connection refused" {
			t.Fatalf("%+v", m)
		}
	})
	t.Run("RF", func(t *testing.T) {
		r, err := c.RF(ctx, Window{From: from, To: to, Region: "OAK,SFO"})
		if err != nil {
			t.Fatal(err)
		}
		if r.TotalPackets != 120 || len(r.PacketsPerHour) != 2 || r.SignalOverTime[0].AvgSnr != 6.25 || r.PayloadTypes[1].Name != "GRP_TXT" {
			t.Fatalf("%+v", r)
		}
		q := rec.lastReq().URL.Query()
		if q.Get("from") != "2026-11-23T21:00:00Z" || q.Get("to") != "2026-11-24T00:00:00Z" || q.Get("region") != "OAK,SFO" {
			t.Fatalf("query = %v", q)
		}
	})
	t.Run("RF omits zero values", func(t *testing.T) {
		if _, err := c.RF(ctx, Window{}); err != nil {
			t.Fatal(err)
		}
		if raw := rec.lastReq().URL.RawQuery; raw != "" {
			t.Fatalf("query = %q, want empty", raw)
		}
	})
	t.Run("Retransmissions", func(t *testing.T) {
		r, err := c.Retransmissions(ctx, Window{From: from, To: to})
		if err != nil {
			t.Fatal(err)
		}
		if r.BucketSeconds != 3600 || len(r.Buckets) < 2 || r.Buckets[0].Start != "2026-11-23T23:00:00Z" || r.Buckets[0].RepeaterSum != 428 {
			t.Fatalf("%+v", r)
		}
		if rec.lastReq().URL.Query().Has("region") {
			t.Fatal("empty region was sent")
		}
	})
	t.Run("ObserverMetrics", func(t *testing.T) {
		m, err := c.ObserverMetrics(ctx, testObserverID, from, to, "5m")
		if err != nil {
			t.Fatal(err)
		}
		if m.ObserverID != testObserverID || m.ObserverName == nil || *m.ObserverName != "Kennedy Repeater" || len(m.Reboots) != 1 ||
			m.Metrics[0].NoiseFloor == nil || *m.Metrics[0].NoiseFloor != -112.5 || m.Metrics[1].RecvErrorRate != nil {
			t.Fatalf("%+v", m)
		}
		q := rec.lastReq().URL.Query()
		if q.Get("since") != "2026-11-23T21:00:00Z" || q.Get("until") != "2026-11-24T00:00:00Z" || q.Get("resolution") != "5m" {
			t.Fatalf("query = %v", q)
		}
	})
	t.Run("NodeAnalytics uppercase pubkey", func(t *testing.T) {
		a, err := c.NodeAnalytics(ctx, strings.ToUpper(testPubkey), 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(a.ActivityTimeline) != 1 || a.ActivityTimeline[0].Count != 1 || a.ActivityTimeline[0].Certain == nil || *a.ActivityTimeline[0].Certain != 1 {
			t.Fatalf("%+v", a)
		}
		if got := rec.lastReq().URL.Query().Get("days"); got != "7" {
			t.Fatalf("days = %q", got)
		}
	})
	t.Run("NodeBattery", func(t *testing.T) {
		b, err := c.NodeBattery(ctx, testPubkey, 7)
		if err != nil {
			t.Fatal(err)
		}
		if b.PublicKey != testPubkey || len(b.Samples) != 3 || b.Samples[2].BatteryMv != 3480 || b.Status != "low" || b.Thresholds.LowMv != 3500 {
			t.Fatalf("%+v", b)
		}
	})
	t.Run("Nodes", func(t *testing.T) {
		n, err := c.Nodes(ctx, NodeQuery{Limit: 50, Role: "repeater", Search: "a&b=c"})
		if err != nil {
			t.Fatal(err)
		}
		if n.Total != 200 || !n.HasMore || len(n.Nodes) == 0 || n.Nodes[0].Name != "KN6PLV-BrkOxfLA-Yebes" || n.Nodes[0].Lat == nil {
			t.Fatalf("%+v", n)
		}
		q := rec.lastReq().URL.Query()
		if q.Get("limit") != "50" || q.Get("role") != "repeater" || q.Get("search") != "a&b=c" || q.Has("region") {
			t.Fatalf("query = %v", q)
		}
	})
	t.Run("Observers", func(t *testing.T) {
		o, err := c.Observers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Observers) == 0 || o.Observers[0].ID != testObserverID || o.Observers[0].IATA == nil || *o.Observers[0].IATA != "OAK" || o.Observers[0].PacketCount != 20771 {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("Regions", func(t *testing.T) {
		r, err := c.Regions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(r) != 4 || r["OAK"] != "OAK" {
			t.Fatalf("%+v", r)
		}
	})
}

func TestClientBasePathKept(t *testing.T) {
	var path atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		serveJSON(w, 200, []byte(`{"ready":true}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL+"/corescope/", nil)
	if _, err := c.Healthz(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := path.Load(); got != "/corescope/api/healthz" {
		t.Fatalf("path = %v", got)
	}
}

func TestClientHeaders(t *testing.T) {
	srv, rec := fixtureServer(t)

	check := func(t *testing.T, wantAuth string) {
		t.Helper()
		r := rec.lastReq()
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}
		if r.Header.Get("X-API-Key") != "" {
			t.Error("X-API-Key was sent")
		}
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("User-Agent") != "corescope-grafana-datasource" {
			t.Errorf("headers = %v", r.Header)
		}
	}

	t.Run("basic", func(t *testing.T) {
		c := newTestClient(t, srv.URL, func(s *models.PluginSettings) {
			s.BasicAuthUser = "grafana"
			s.Secrets = &models.SecretPluginSettings{BasicAuthPassword: "pw:1"}
		})
		if _, err := c.Stats(context.Background()); err != nil {
			t.Fatal(err)
		}
		check(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("grafana:pw:1")))
	})
	t.Run("bearer", func(t *testing.T) {
		c := newTestClient(t, srv.URL, func(s *models.PluginSettings) {
			s.Secrets = &models.SecretPluginSettings{BearerToken: "tok123"}
		})
		if _, err := c.Stats(context.Background()); err != nil {
			t.Fatal(err)
		}
		check(t, "Bearer tok123")
	})
	t.Run("none", func(t *testing.T) {
		c := newTestClient(t, srv.URL, nil)
		if _, err := c.Stats(context.Background()); err != nil {
			t.Fatal(err)
		}
		check(t, "")
	})
}

func TestClientRejectsInvalidSettings(t *testing.T) {
	_, err := NewClient(&models.PluginSettings{URL: "ftp://host"})
	if !errors.Is(err, models.ErrURLScheme) {
		t.Fatalf("err = %v", err)
	}
}

func TestClientRedirectNotFollowed(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		serveJSON(w, 200, []byte(`{}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/stats", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, func(s *models.PluginSettings) {
		s.Secrets = &models.SecretPluginSettings{BearerToken: "tok"}
	})
	_, err := c.Stats(context.Background())
	wantError(t, err, http.StatusFound, "upstream redirected; check the URL")
	if n := otherHits.Load(); n != 0 {
		t.Fatalf("redirect target got %d requests", n)
	}
}

func TestClientSPAFallbackIsError(t *testing.T) {
	srv, _ := fixtureServer(t)
	c := newTestClient(t, srv.URL, nil)
	// Valid observer id, but no fixture for it: the server answers HTML.
	_, err := c.ObserverMetrics(context.Background(), "unknown", time.Time{}, time.Time{}, "")
	wantError(t, err, http.StatusBadGateway, "upstream returned 200 text/html; the URL may not point at a CoreScope API")
}

func TestClientErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		status  int
		msg     string
	}{
		{
			name:    "json error field",
			handler: func(w http.ResponseWriter, r *http.Request) { serveJSON(w, 500, fixture(t, "error_500.json")) },
			status:  500, msg: "no such table: observer_metrics",
		},
		{
			name: "non-JSON 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(500)
				_, _ = w.Write([]byte(`{"error":"secret internals"} at /var/lib/db`))
			},
			status: 500, msg: "Internal Server Error",
		},
		{
			name: "control chars stripped and truncated",
			handler: func(w http.ResponseWriter, r *http.Request) {
				serveJSON(w, 400, []byte(`{"error":"bad\u001b[31m\r\n`+strings.Repeat("é", 300)+`"}`))
			},
			status: 400, msg: "bad[31m" + strings.Repeat("é", 200-len("bad[31m")),
		},
		{
			name:    "empty error field",
			handler: func(w http.ResponseWriter, r *http.Request) { serveJSON(w, 404, []byte(`{"error":""}`)) },
			status:  404, msg: "Not Found",
		},
		{
			name:    "invalid JSON 2xx",
			handler: func(w http.ResponseWriter, r *http.Request) { serveJSON(w, 200, []byte(`{"totalPackets":`)) },
			status:  502, msg: "invalid JSON from upstream",
		},
		{
			name: "2xx without content type",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = nil
				_, _ = w.Write([]byte(`{}`))
			},
			status: 502, msg: "upstream returned 200 without a JSON content type; the URL may not point at a CoreScope API",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			c := newTestClient(t, srv.URL, nil)
			_, err := c.Stats(context.Background())
			e := wantError(t, err, tt.status, tt.msg)
			if e.Path != "/api/stats" {
				t.Errorf("Path = %q", e.Path)
			}
		})
	}
}

func TestClientOversizedBody(t *testing.T) {
	big := append([]byte(`{"totalPackets":1,"pad":"`), bytes.Repeat([]byte("x"), maxBodyBytes)...)
	big = append(big, `"}`...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(w, 200, big)
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	_, err := c.Stats(context.Background())
	wantError(t, err, http.StatusBadGateway, "upstream response too large")

	// /api/nodes has the larger cap, so the same body decodes there.
	if _, err := c.Nodes(context.Background(), NodeQuery{}); err != nil {
		t.Fatalf("Nodes with %d-byte body: %v", len(big), err)
	}
}

func TestClientGzip(t *testing.T) {
	body := fixture(t, "stats.json")
	var sawGzip atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			serveJSON(w, 200, body)
			return
		}
		sawGzip.Store(true)
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(body)
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	s, err := c.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sawGzip.Load() || s.TotalPackets != 499 {
		t.Fatalf("gzip=%v stats=%+v", sawGzip.Load(), s)
	}
}

func TestClientRetries503ThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			serveJSON(w, 503, []byte(`{"error":"warming up"}`))
			return
		}
		serveJSON(w, 200, []byte(`{"ready":true}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	h, err := c.Healthz(context.Background())
	if err != nil || !h.Ready {
		t.Fatalf("%+v %v", h, err)
	}
	if n := hits.Load(); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestClient503ForeverFailsWithinBudget(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 503, []byte(`{"error":"warming up"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, func(s *models.PluginSettings) { s.TimeoutSeconds = 1 })
	start := time.Now()
	_, err := c.RF(context.Background(), Window{})
	elapsed := time.Since(start)
	wantError(t, err, 503, "warming up")
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("took %v, budget is 1s", elapsed)
	}
	if n := hits.Load(); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestClientRetryAfterBeyondBudgetFailsFast(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "30")
		serveJSON(w, 503, []byte(`{"error":"warming up"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, func(s *models.PluginSettings) { s.TimeoutSeconds = 2 })
	start := time.Now()
	_, err := c.RF(context.Background(), Window{})
	wantError(t, err, 503, "warming up")
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("waited %v despite Retry-After beyond budget", elapsed)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}

func TestClientRetryAfterHonored(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			serveJSON(w, 503, []byte(`{"error":"warming up"}`))
			return
		}
		serveJSON(w, 200, []byte(`{"ready":true}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	start := time.Now()
	if _, err := c.Healthz(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("retried after %v, Retry-After was 1s", elapsed)
	}
}

func TestClient404NotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 404, []byte(`{"error":"node not found"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	_, err := c.NodeBattery(context.Background(), testPubkey, 7)
	wantError(t, err, 404, "node not found")
	if n := hits.Load(); n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}

func TestClientContextCancelStopsRetries(t *testing.T) {
	var hits atomic.Int32
	first := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 503, []byte(`{"error":"warming up"}`))
		once.Do(func() { close(first) })
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-first; cancel() }()
	start := time.Now()
	_, err := c.Stats(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v", elapsed)
	}
	time.Sleep(600 * time.Millisecond) // longer than the first two backoffs
	if n := hits.Load(); n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}

func TestClientConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	c := newTestClient(t, "http://"+addr, func(s *models.PluginSettings) {
		s.Secrets = &models.SecretPluginSettings{BearerToken: "tok"}
	})
	start := time.Now()
	_, err = c.Stats(context.Background())
	e := wantError(t, err, http.StatusBadGateway, "connection refused")
	if strings.Contains(e.Error(), "tok") {
		t.Fatal("error leaks the token")
	}
	// Two backoffs of at least 125ms and 250ms prove it retried.
	if elapsed := time.Since(start); elapsed < 375*time.Millisecond {
		t.Fatalf("returned after %v; connection refused should be retried", elapsed)
	}
}

func TestClientInvalidPathSegmentsMakeNoRequest(t *testing.T) {
	srv, rec := fixtureServer(t)
	c := newTestClient(t, srv.URL, nil)
	ctx := context.Background()
	for _, id := range []string{"", "..", ".", "a/b", "x?y", "a%2Fb", "a b", "a#b", strings.Repeat("a", 129)} {
		_, err := c.ObserverMetrics(ctx, id, time.Time{}, time.Time{}, "")
		e := wantError(t, err, 400, "invalid observer id")
		if id != "" && strings.Contains(e.Error(), id) {
			t.Errorf("error %q echoes the input", e.Error())
		}
	}
	for _, pk := range []string{"", testPubkey[:63], testPubkey + "0", "../" + testPubkey[3:], strings.Replace(testPubkey, "9", "g", 1)} {
		if _, err := c.NodeAnalytics(ctx, pk, 7); err == nil {
			t.Errorf("NodeAnalytics(%q) succeeded", pk)
		} else {
			wantError(t, err, 400, "invalid pubkey")
		}
		if _, err := c.NodeBattery(ctx, pk, 7); err == nil {
			t.Errorf("NodeBattery(%q) succeeded", pk)
		} else {
			wantError(t, err, 400, "invalid pubkey")
		}
	}
	if n := rec.hits.Load(); n != 0 {
		t.Fatalf("%d upstream requests made for invalid input", n)
	}
	// The 128-char boundary is accepted.
	_, _ = c.ObserverMetrics(ctx, strings.Repeat("a", 128), time.Time{}, time.Time{}, "")
	if n := rec.hits.Load(); n != 1 {
		t.Fatalf("128-char id: hits = %d, want 1", n)
	}
}

func TestClientSingleflightDedupes(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		serveJSON(w, 200, fixture(t, "stats.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	c.cache = newRespCache(0, cacheMaxEntries, time.Now) // cache off: only singleflight can dedupe

	var wg sync.WaitGroup
	results := make([]*Stats, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = c.Stats(context.Background())
		}()
		if i == 0 {
			for hits.Load() == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	time.Sleep(100 * time.Millisecond) // let the second call join the flight
	close(release)
	wg.Wait()
	for i := range 2 {
		if errs[i] != nil || results[i].TotalPackets != 499 {
			t.Fatalf("call %d: %+v %v", i, results[i], errs[i])
		}
	}
	if results[0] == results[1] {
		t.Fatal("callers share one decoded value")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hits = %d, want 1", n)
	}
}

func TestClientSingleflightFollowerSurvivesLeaderCancel(t *testing.T) {
	var hits atomic.Int32
	firstEnded := make(chan struct{})
	var secondBeforeFirstEnded atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			<-r.Context().Done() // the leader's request hangs until it is cancelled
			close(firstEnded)
			return
		}
		select {
		case <-firstEnded:
		default:
			secondBeforeFirstEnded.Store(true)
		}
		serveJSON(w, 200, []byte(`{"ready":true}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	var retries atomic.Int32
	c.sharedRetryHook = func() { retries.Add(1) }

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() { _, err := c.Healthz(leaderCtx); leaderDone <- err }()
	for hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	followerDone := make(chan error, 1)
	go func() { _, err := c.Healthz(context.Background()); followerDone <- err }()
	time.Sleep(100 * time.Millisecond) // let the follower join the leader's flight
	cancel()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v", err)
	}
	if err := <-followerDone; err != nil {
		t.Fatalf("follower err = %v", err)
	}
	// The follower joined the flight (no request of its own while the
	// leader's was blocked), saw the leader's cancel, and re-fetched once.
	if n := retries.Load(); n != 1 {
		t.Fatalf("shared retries = %d, want 1", n)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("upstream hits = %d, want 2", n)
	}
	if secondBeforeFirstEnded.Load() {
		t.Fatal("second request arrived while the leader's was still blocked")
	}
}

func TestClientCloseClosesIdleConnections(t *testing.T) {
	var closed atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(w, 200, []byte(`{"ready":true}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateClosed {
			closed.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	s := &models.PluginSettings{URL: srv.URL, TimeoutSeconds: 5}
	c, err := NewClient(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Healthz(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // the keep-alive conn is now idle
	if n := closed.Load(); n != 0 {
		t.Fatalf("connection closed before Close: %d", n)
	}
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for closed.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Close did not close the idle connection")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestClientGzipBombCapped(t *testing.T) {
	// 9 MiB of zeros inside a JSON string inflates past the 8 MiB cap but
	// compresses to a few KiB, so only a post-decompression limit catches it.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(`{"totalPackets":1,"pad":"`))
	_, _ = zw.Write(bytes.Repeat([]byte("0"), 9<<20))
	_, _ = zw.Write([]byte(`"}`))
	_ = zw.Close()
	if buf.Len() > 64<<10 {
		t.Fatalf("compressed size %d, want a small bomb", buf.Len())
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	_, err := c.Stats(context.Background())
	wantError(t, err, http.StatusBadGateway, "upstream response too large")
}

func TestClientCacheTTL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 200, fixture(t, "stats.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	var mu sync.Mutex
	now := time.Unix(1_800_000_000, 0)
	c.cache = newRespCache(cacheTTL, cacheMaxEntries, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	ctx := context.Background()

	s1, err := c.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s1.TotalPackets = -1 // a caller mutating its result must not affect others
	advance(4 * time.Second)
	s2, err := c.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s2.TotalPackets != 499 {
		t.Fatalf("cached value was mutated: %d", s2.TotalPackets)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("hits within TTL = %d, want 1", n)
	}
	advance(2 * time.Second)
	if _, err := c.Stats(ctx); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("hits after TTL = %d, want 2", n)
	}
	// A different URL (query) is a different key.
	if _, err := c.Nodes(ctx, NodeQuery{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Nodes(ctx, NodeQuery{Limit: 2}); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 4 {
		t.Fatalf("hits = %d, want 4", n)
	}
}

func TestClientErrorsNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 500, fixture(t, "error_500.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	for range 2 {
		if _, err := c.Stats(context.Background()); err == nil {
			t.Fatal("want error")
		}
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("hits = %d, want 2", n)
	}
}

func TestRespCacheBounded(t *testing.T) {
	now := time.Unix(0, 0)
	rc := newRespCache(time.Minute, cacheMaxEntries, func() time.Time { return now })
	for i := range 1000 {
		now = now.Add(time.Millisecond)
		rc.put("k"+itoa(i), []byte("v"))
	}
	if n := rc.len(); n != cacheMaxEntries {
		t.Fatalf("len = %d, want %d", n, cacheMaxEntries)
	}
	if _, ok := rc.get("k999"); !ok {
		t.Fatal("newest entry evicted")
	}
	if _, ok := rc.get("k0"); ok {
		t.Fatal("oldest entry kept")
	}
}

func TestClientOversizedBodyNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(w, 200, fixture(t, "stats.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, nil)
	c.cache.maxEntryBytes = 100 // stats.json is larger
	for range 2 {
		s, err := c.Stats(context.Background())
		if err != nil || s.TotalPackets != 499 {
			t.Fatalf("%+v %v", s, err)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("hits = %d, want 2 (oversized body must not be cached)", n)
	}
	if n, b := c.cache.len(), c.cache.size(); n != 0 || b != 0 {
		t.Fatalf("cache len=%d bytes=%d, want 0/0", n, b)
	}
}

func TestRespCacheByteBound(t *testing.T) {
	now := time.Unix(0, 0)
	rc := newRespCache(time.Minute, cacheMaxEntries, func() time.Time { return now })
	rc.maxBytes = 10
	rc.maxEntryBytes = 10

	rc.put("a", []byte("aaaa")) // expires first
	now = now.Add(time.Second)
	rc.put("b", []byte("bbbb"))
	now = now.Add(time.Second)
	rc.put("c", []byte("cccc")) // 12 > 10: evicts a only
	if _, ok := rc.get("a"); ok {
		t.Fatal("soonest-expiring entry kept")
	}
	if _, ok := rc.get("b"); !ok {
		t.Fatal("b evicted; only one eviction was needed")
	}
	if n, b := rc.len(), rc.size(); n != 2 || b != 8 {
		t.Fatalf("len=%d bytes=%d, want 2/8", n, b)
	}

	// An entry over the per-entry cap is dropped and supersedes the old body.
	rc.put("b", []byte("bbbbbbbbbbb"))
	if _, ok := rc.get("b"); ok {
		t.Fatal("oversized overwrite left the stale body")
	}
	if n, b := rc.len(), rc.size(); n != 1 || b != 4 {
		t.Fatalf("len=%d bytes=%d, want 1/4", n, b)
	}

	// A body that needs every slot evicts all others.
	rc.put("d", []byte("dddddddddd"))
	if n, b := rc.len(), rc.size(); n != 1 || b != 10 {
		t.Fatalf("len=%d bytes=%d, want 1/10", n, b)
	}
}

func TestRespCacheBytesAfterOverwriteAndExpiry(t *testing.T) {
	now := time.Unix(0, 0)
	rc := newRespCache(5*time.Second, cacheMaxEntries, func() time.Time { return now })

	rc.put("k", []byte("123456"))
	rc.put("k", []byte("12")) // overwrite shrinks
	if n, b := rc.len(), rc.size(); n != 1 || b != 2 {
		t.Fatalf("after overwrite len=%d bytes=%d, want 1/2", n, b)
	}
	if got, _ := rc.get("k"); string(got) != "12" {
		t.Fatalf("get = %q", got)
	}
	rc.put("j", []byte("abc"))
	if b := rc.size(); b != 5 {
		t.Fatalf("bytes = %d, want 5", b)
	}

	now = now.Add(5 * time.Second)
	if _, ok := rc.get("k"); ok {
		t.Fatal("expired entry returned")
	}
	if n, b := rc.len(), rc.size(); n != 1 || b != 3 {
		t.Fatalf("after expiry len=%d bytes=%d, want 1/3", n, b)
	}

	// Expired entries are purged before live ones are evicted.
	rc.max = 2
	rc.put("x", []byte("x"))
	now = now.Add(time.Second)
	rc.put("y", []byte("yy")) // j and x full the cache; j is expired
	if _, ok := rc.get("x"); !ok {
		t.Fatal("live entry evicted while an expired one was held")
	}
	if n, b := rc.len(), rc.size(); n != 2 || b != 3 {
		t.Fatalf("len=%d bytes=%d, want 2/3", n, b)
	}
}

func TestSanitizeMessage(t *testing.T) {
	// NUL, tab, a bidi override (Cf) and invalid UTF-8 are all dropped.
	if got := sanitizeMessage("  a\x00b\tc\u202ed\xff  "); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}
