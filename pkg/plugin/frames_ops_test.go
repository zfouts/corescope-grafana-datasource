package plugin

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

func loadOpsFixture(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile("../corescope/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

type opsFieldSpec struct {
	name string
	typ  data.FieldType
	unit string
}

func checkOpsFields(t *testing.T, f *data.Frame, want []opsFieldSpec) {
	t.Helper()
	if len(f.Fields) != len(want) {
		t.Fatalf("got %d fields, want %d", len(f.Fields), len(want))
	}
	for i, w := range want {
		g := f.Fields[i]
		if g.Name != w.name || g.Type() != w.typ {
			t.Errorf("field %d = %s %s, want %s %s", i, g.Name, g.Type(), w.name, w.typ)
		}
		unit := ""
		if g.Config != nil {
			unit = g.Config.Unit
		}
		if unit != w.unit {
			t.Errorf("field %s unit = %q, want %q", w.name, unit, w.unit)
		}
	}
}

func opsField(t *testing.T, f *data.Frame, name string) *data.Field {
	t.Helper()
	fl, _ := f.FieldByName(name)
	if fl == nil {
		t.Fatalf("no field %s", name)
	}
	return fl
}

func TestStatsFrame(t *testing.T) {
	var s corescope.Stats
	loadOpsFixture(t, "stats.json", &s)
	f := StatsFrame(&s)
	if f.Name != "stats" || f.Rows() != 1 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	i, fl, mb, str := data.FieldTypeInt64, data.FieldTypeFloat64, "decmbytes", data.FieldTypeString
	checkOpsFields(t, f, []opsFieldSpec{
		{"totalPackets", i, ""}, {"totalTransmissions", i, ""}, {"totalObservations", i, ""},
		{"totalNodes", i, ""}, {"totalNodesAllTime", i, ""}, {"totalObservers", i, ""},
		{"packetsLastHour", i, ""}, {"packetsLast24h", i, ""}, {"repeaters", i, ""},
		{"rooms", i, ""}, {"companions", i, ""}, {"sensors", i, ""}, {"signatureDrops", i, ""},
		{"storeDataMB", fl, mb}, {"processRSSMB", fl, mb}, {"goHeapInuseMB", fl, mb}, {"goSysMB", fl, mb},
		{"version", str, ""}, {"commit", str, ""},
	})
	if v := opsField(t, f, "totalPackets").At(0); v != int64(499) {
		t.Errorf("totalPackets = %v", v)
	}
	if v := opsField(t, f, "totalNodes").At(0); v != int64(200) {
		t.Errorf("totalNodes = %v", v)
	}
	if v := opsField(t, f, "storeDataMB").At(0); v != 1.3 {
		t.Errorf("storeDataMB = %v", v)
	}
	if v := opsField(t, f, "version").At(0); v != "unknown" {
		t.Errorf("version = %v", v)
	}
}

func TestStatsFrameNilTransmissions(t *testing.T) {
	f := StatsFrame(&corescope.Stats{Version: "v\x07"})
	if v := opsField(t, f, "totalTransmissions").At(0); v != int64(0) {
		t.Errorf("totalTransmissions = %v", v)
	}
	if v := opsField(t, f, "version").At(0); v != "v" {
		t.Errorf("version = %q", v)
	}
}

func TestHealthFrame(t *testing.T) {
	var h corescope.Health
	loadOpsFixture(t, "health.json", &h)
	f := HealthFrame(&h)
	if f.Name != "health" || f.Rows() != 1 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	i, fl, ms, mb := data.FieldTypeInt64, data.FieldTypeFloat64, "ms", "decmbytes"
	checkOpsFields(t, f, []opsFieldSpec{
		{"uptime", i, "s"},
		{"rssMB", fl, mb}, {"heapUsedMB", fl, mb}, {"heapTotalMB", fl, mb},
		{"eventLoopLagMs", fl, ms}, {"eventLoopP95Ms", fl, ms}, {"eventLoopP99Ms", fl, ms}, {"avgRequestMs", fl, ms},
		{"cacheHitRate", fl, "percentunit"},
		{"cacheEntries", i, ""}, {"wsClients", i, ""}, {"wsRejectedDeny", i, ""}, {"wsRejectedRate", i, ""},
		{"wsRejectedConnCap", i, ""}, {"packetStorePackets", i, ""}, {"totalRequests", i, ""}, {"slowQueries", i, ""},
		{"status", data.FieldTypeString, ""},
	})
	if v := opsField(t, f, "uptime").At(0); v != int64(6) {
		t.Errorf("uptime = %v", v)
	}
	if v := opsField(t, f, "status").At(0); v != "ok" {
		t.Errorf("status = %v", v)
	}
	if v := opsField(t, f, "totalRequests").At(0); v != int64(5) {
		t.Errorf("totalRequests = %v", v)
	}
	if v := opsField(t, f, "avgRequestMs").At(0); v != 0.6 {
		t.Errorf("avgRequestMs = %v", v)
	}
}

func TestEndpointsFrame(t *testing.T) {
	var p corescope.Perf
	loadOpsFixture(t, "perf.json", &p)
	f := EndpointsFrame(&p)
	if f.Name != "endpoints" || f.Rows() != 5 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeTable {
		t.Errorf("meta = %+v", f.Meta)
	}
	fl, ms := data.FieldTypeFloat64, "ms"
	checkOpsFields(t, f, []opsFieldSpec{
		{"endpoint", data.FieldTypeString, ""}, {"count", data.FieldTypeInt64, ""},
		{"avgMs", fl, ms}, {"p50Ms", fl, ms}, {"p95Ms", fl, ms}, {"maxMs", fl, ms},
	})
	// count desc (healthz=2 first), then endpoint asc.
	want := []string{"/api/healthz", "/api/health", "/api/nodes", "/api/observers", "/api/stats"}
	for i, w := range want {
		if got := opsField(t, f, "endpoint").At(i); got != w {
			t.Errorf("row %d endpoint = %v, want %v", i, got, w)
		}
	}
	if v := opsField(t, f, "count").At(0); v != int64(2) {
		t.Errorf("count[0] = %v", v)
	}
	if v := opsField(t, f, "avgMs").At(3); v != 1.2 {
		t.Errorf("avgMs[3] = %v", v)
	}
}

func TestEndpointsFrameEmpty(t *testing.T) {
	f := EndpointsFrame(&corescope.Perf{})
	if f.Rows() != 0 || len(f.Fields) != 6 {
		t.Fatalf("rows=%d fields=%d", f.Rows(), len(f.Fields))
	}
}

func TestMqttFrame(t *testing.T) {
	var m corescope.MqttStatus
	loadOpsFixture(t, "mqtt_status_sources.json", &m)
	f := MqttFrame(&m)
	if f.Name != "mqtt" || f.Rows() != 2 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeTable {
		t.Errorf("meta = %+v", f.Meta)
	}
	i, ts, s := data.FieldTypeInt64, data.FieldTypeNullableTime, data.FieldTypeString
	checkOpsFields(t, f, []opsFieldSpec{
		{"name", s, ""}, {"broker", s, ""}, {"lastError", s, ""}, {"connected", data.FieldTypeBool, ""},
		{"lastConnect", ts, ""}, {"lastDisconnect", ts, ""}, {"lastPacket", ts, ""},
		{"connectCount", i, ""}, {"disconnectCount", i, ""}, {"packetsTotal", i, ""}, {"packetsLast5m", i, ""},
	})
	if v := opsField(t, f, "name").At(0); v != "primary" {
		t.Errorf("name[0] = %v", v)
	}
	if v := opsField(t, f, "connected").At(1); v != false {
		t.Errorf("connected[1] = %v", v)
	}
	// primary: lastDisconnectUnix 0 → nil; lastConnect set.
	if v := opsField(t, f, "lastDisconnect").At(0).(*time.Time); v != nil {
		t.Errorf("lastDisconnect[0] = %v, want nil", v)
	}
	if v := opsField(t, f, "lastConnect").At(0).(*time.Time); v == nil || !v.Equal(time.Unix(1795480000, 0)) || v.Location() != time.UTC {
		t.Errorf("lastConnect[0] = %v", v)
	}
	if v := opsField(t, f, "lastDisconnect").At(1).(*time.Time); v == nil || v.Unix() != 1795470000 {
		t.Errorf("lastDisconnect[1] = %v", v)
	}
	if v := opsField(t, f, "lastError").At(1); v != "dial tcp: connection refused" {
		t.Errorf("lastError[1] = %v", v)
	}
	if v := opsField(t, f, "lastError").At(0); v != "" {
		t.Errorf("lastError[0] = %v", v)
	}
	if v := opsField(t, f, "packetsTotal").At(0); v != int64(12034) {
		t.Errorf("packetsTotal[0] = %v", v)
	}
}

func TestMqttFrameNoSources(t *testing.T) {
	var m corescope.MqttStatus
	loadOpsFixture(t, "mqtt_status.json", &m)
	f := MqttFrame(&m)
	if f.Rows() != 0 || len(f.Fields) != 11 {
		t.Fatalf("rows=%d fields=%d", f.Rows(), len(f.Fields))
	}
}
