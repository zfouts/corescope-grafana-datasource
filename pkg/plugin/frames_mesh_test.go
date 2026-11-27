package plugin

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

func meshFixture(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile("../corescope/testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func meshFieldNames(f *data.Frame) []string {
	out := []string{}
	for _, fl := range f.Fields {
		out = append(out, fl.Name)
	}
	return out
}

func meshWantNames(t *testing.T, f *data.Frame, want ...string) {
	t.Helper()
	got := meshFieldNames(f)
	if len(got) != len(want) {
		t.Fatalf("fields %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fields %v want %v", got, want)
		}
	}
}

func meshTime(s string) time.Time {
	t, _ := corescope.ParseTime(s)
	return t
}

func TestMeshPacketsPerHourFrame(t *testing.T) {
	var rf corescope.RFAnalytics
	meshFixture(t, "analytics_rf", &rf)
	f := PacketsPerHourFrame(&rf)
	if f.Name != "packetsPerHour" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "time", "packets", "avgSnr")
	if f.Rows() != 3 {
		t.Fatalf("rows %d", f.Rows())
	}
	if got := f.Fields[0].At(0).(time.Time); !got.Equal(meshTime("2026-11-23T23")) || got.Location() != time.UTC {
		t.Fatalf("time %v", got)
	}
	if f.Fields[1].At(1).(int64) != 246 {
		t.Fatal("packets")
	}
	for i := 0; i < 3; i++ {
		if f.Fields[2].At(i).(*float64) != nil {
			t.Fatal("avgSnr should be nil without signal data")
		}
	}
}

func TestMeshPacketsPerHourSNRJoinAndOrder(t *testing.T) {
	var rf corescope.RFAnalytics
	meshFixture(t, "analytics_rf_signal", &rf)
	// reverse input order to prove sorting
	rf.PacketsPerHour[0], rf.PacketsPerHour[1] = rf.PacketsPerHour[1], rf.PacketsPerHour[0]
	rf.PacketsPerHour = append(rf.PacketsPerHour, corescope.HourlyCount{Hour: "garbage", Count: 5})
	f := PacketsPerHourFrame(&rf)
	if f.Rows() != 2 {
		t.Fatalf("rows %d", f.Rows())
	}
	if !f.Fields[0].At(0).(time.Time).Equal(meshTime("2026-11-23T22")) {
		t.Fatal("not ascending")
	}
	if f.Fields[2].At(0).(*float64) != nil {
		t.Fatal("22h has no snr")
	}
	if p := f.Fields[2].At(1).(*float64); p == nil || *p != 6.25 {
		t.Fatalf("snr %v", p)
	}
	if f.Fields[1].At(1).(int64) != 80 {
		t.Fatal("packets")
	}
}

func TestMeshPayloadTypesFrame(t *testing.T) {
	var rf corescope.RFAnalytics
	meshFixture(t, "analytics_rf", &rf)
	f := PayloadTypesFrame(&rf)
	if f.Name != "payloadTypes" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "type", "count")
	if f.Rows() != 9 {
		t.Fatalf("rows %d", f.Rows())
	}
	if f.Fields[0].At(0).(string) != "GRP_TXT" || f.Fields[1].At(0).(int64) != 166 {
		t.Fatal("first row")
	}
	if f.Fields[0].At(8).(string) != "CONTROL" {
		t.Fatal("last row")
	}
	// tie-break by name, empty name fallback, control chars stripped
	f = PayloadTypesFrame(&corescope.RFAnalytics{PayloadTypes: []corescope.PayloadTypeCount{
		{Type: 1, Name: "B", Count: 5},
		{Type: 2, Name: "A\x00", Count: 5},
		{Type: 13, Name: "", Count: 9},
	}})
	got := []string{f.Fields[0].At(0).(string), f.Fields[0].At(1).(string), f.Fields[0].At(2).(string)}
	want := []string{"type 13", "A", "B"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestMeshRetransmissionsFrame(t *testing.T) {
	var r corescope.Retransmissions
	meshFixture(t, "analytics_retransmissions", &r)
	r.Buckets = append(r.Buckets, corescope.RetransmissionBucket{Start: "nope", Packets: 1})
	f := RetransmissionsFrame(&r)
	if f.Name != "retransmissions" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "time", "packets", "repeaterSum", "observers", "avgRepeaters")
	if f.Rows() != 3 {
		t.Fatalf("rows %d", f.Rows())
	}
	if f.Fields[1].At(0).(int64) != 52 || f.Fields[2].At(0).(int64) != 428 || f.Fields[3].At(0).(int64) != 14 {
		t.Fatal("row 0 values")
	}
	if f.Fields[4].At(2).(float64) != 7.8 {
		t.Fatal("avgRepeaters")
	}
	if !f.Fields[0].At(2).(time.Time).Equal(meshTime("2026-11-24T01:00:00Z")) {
		t.Fatal("time")
	}
}

func TestMeshObserverMetricsFrame(t *testing.T) {
	var m corescope.ObserverMetrics
	meshFixture(t, "observers_ID_metrics", &m)
	f := ObserverMetricsFrame(&m)
	if f.Name != "Kennedy Repeater" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "time", "noiseFloor", "batteryMv", "txAirtimePct", "rxAirtimePct", "recvErrorRate", "packetsSent", "packetsRecv", "reboot")
	units := map[string]string{"noiseFloor": "dBm", "batteryMv": "mvolt", "txAirtimePct": "percent", "rxAirtimePct": "percent", "recvErrorRate": "percent"}
	for _, fl := range f.Fields {
		want := units[fl.Name]
		got := ""
		if fl.Config != nil {
			got = fl.Config.Unit
		}
		if got != want {
			t.Errorf("%s unit %q want %q", fl.Name, got, want)
		}
	}
	if f.Rows() != 4 {
		t.Fatalf("rows %d", f.Rows())
	}
	if _, ok := f.Fields[2].At(0).(*int64); !ok {
		t.Fatal("batteryMv must be *int64")
	}
	if v := f.Fields[2].At(0).(*int64); v == nil || *v != 4012 {
		t.Fatal("battery")
	}
	if v := f.Fields[1].At(3).(*float64); v == nil || *v != -113.25 {
		t.Fatal("noise")
	}
	// row 1: recv_error_rate nil; row 3: 0.0 preserved as non-nil zero
	if f.Fields[5].At(1).(*float64) != nil {
		t.Fatal("null recvErrorRate must stay nil")
	}
	if v := f.Fields[5].At(3).(*float64); v == nil || *v != 0 {
		t.Fatal("zero must stay non-nil")
	}
	// reboot row: every reading nil (nil pointers, typed per field)
	for i := 1; i <= 7; i++ {
		if !reflect.ValueOf(f.Fields[i].At(2)).IsNil() {
			t.Fatalf("field %d reboot row not nil", i)
		}
	}
	if !f.Fields[8].At(2).(bool) || f.Fields[8].At(0).(bool) {
		t.Fatal("reboot flag")
	}
	if v := f.Fields[6].At(0).(*int64); v == nil || *v != 3 {
		t.Fatal("packetsSent")
	}
}

func TestMeshObserverMetricsNameFallbacks(t *testing.T) {
	n := "Obs\x01Name"
	f := ObserverMetricsFrame(&corescope.ObserverMetrics{ObserverID: "ABC", ObserverName: &n})
	if f.Name != "ObsName" {
		t.Fatal(f.Name)
	}
	f = ObserverMetricsFrame(&corescope.ObserverMetrics{ObserverID: "ABC"})
	if f.Name != "ABC" {
		t.Fatal(f.Name)
	}
	empty := ""
	f = ObserverMetricsFrame(&corescope.ObserverMetrics{ObserverName: &empty})
	if f.Name != "observerMetrics" || f.Rows() != 0 || len(f.Fields) != 9 {
		t.Fatal("empty", f.Name, f.Rows())
	}
	f = ObserverMetricsFrame(&corescope.ObserverMetrics{Metrics: []corescope.MetricsSample{{Timestamp: "bad"}, {Timestamp: "2026-11-23T23:00:00Z"}}})
	if f.Rows() != 1 {
		t.Fatal("unparseable row should be skipped")
	}
}

func TestMeshNodeActivityFrame(t *testing.T) {
	var a corescope.NodeAnalytics
	meshFixture(t, "nodes_PK_analytics", &a)
	from, to := meshTime("2026-11-17T00:00:00Z"), meshTime("2026-11-25T00:00:00Z")
	f := NodeActivityFrame(&a, from, to)
	if f.Name != "nodeActivity" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "time", "packets", "certain")
	if f.Rows() != 1 || f.Fields[1].At(0).(int64) != 1 {
		t.Fatalf("rows %d", f.Rows())
	}
	if v := f.Fields[2].At(0).(*int64); v == nil || *v != 1 {
		t.Fatal("certain")
	}
	// range filtering
	f = NodeActivityFrame(&a, meshTime("2026-11-24T02:00:00Z"), meshTime("2026-11-25T00:00:00Z"))
	if f.Rows() != 0 || len(f.Fields) != 3 {
		t.Fatal("outside range must drop")
	}
	// inclusive bounds
	b := meshTime("2026-11-24T01:00:00Z")
	if NodeActivityFrame(&a, b, b).Rows() != 1 {
		t.Fatal("bounds inclusive")
	}
	// partial first hour: the 01:00 bucket survives from=01:17
	if NodeActivityFrame(&a, meshTime("2026-11-24T01:17:00Z"), to).Rows() != 1 {
		t.Fatal("partial first hour must be kept")
	}
	// label fallback, nil certain, bad timestamp, no timestamp
	lbl, bad := "2026-11-24T01:00:00Z", "zzz"
	f = NodeActivityFrame(&corescope.NodeAnalytics{ActivityTimeline: []corescope.TimeBucket{
		{Label: &lbl, Count: 4},
		{Bucket: &bad, Count: 9},
		{Count: 7},
	}}, from, to)
	if f.Rows() != 1 || f.Fields[2].At(0).(*int64) != nil || f.Fields[1].At(0).(int64) != 4 {
		t.Fatal("label fallback / skip")
	}
	f = NodeActivityFrame(nil, from, to)
	if f == nil || f.Rows() != 0 || len(f.Fields) != 3 {
		t.Fatal("nil input")
	}
}

func TestMeshNodeBatteryFrame(t *testing.T) {
	var b corescope.NodeBattery
	meshFixture(t, "nodes_PK_battery", &b)
	b.Samples = append(b.Samples, corescope.BatterySample{Timestamp: "bad", BatteryMv: 1})
	from, to := meshTime("2026-11-22T00:00:00Z"), meshTime("2026-11-25T00:00:00Z")
	f := NodeBatteryFrame(&b, from, to)
	if f.Name != "nodeBattery" {
		t.Fatal(f.Name)
	}
	meshWantNames(t, f, "time", "batteryMv")
	if f.Fields[1].Config == nil || f.Fields[1].Config.Unit != "mvolt" {
		t.Fatal("unit")
	}
	if f.Rows() != 3 || f.Fields[1].At(0).(int64) != 3720 || f.Fields[1].At(2).(int64) != 3480 {
		t.Fatalf("rows %d", f.Rows())
	}
	c, ok := f.Meta.Custom.(map[string]any)
	if !ok || c["status"] != "low" || c["lowMv"] != int64(3500) || c["criticalMv"] != int64(3000) {
		t.Fatalf("meta %#v", f.Meta)
	}
	f = NodeBatteryFrame(&b, meshTime("2026-11-23T00:00:00Z"), to)
	if f.Rows() != 2 || f.Fields[1].At(0).(int64) != 3610 {
		t.Fatal("range filter")
	}
	f = NodeBatteryFrame(&b, meshTime("2027-01-01T00:00:00Z"), meshTime("2027-01-02T00:00:00Z"))
	if f.Rows() != 0 || len(f.Fields) != 2 || f.Meta == nil {
		t.Fatal("empty range")
	}
	f = NodeBatteryFrame(nil, from, to)
	if f == nil || f.Rows() != 0 {
		t.Fatal("nil input")
	}
}

func TestMeshEmptyInputs(t *testing.T) {
	for name, f := range map[string]*data.Frame{
		"pph": PacketsPerHourFrame(nil),
		"pt":  PayloadTypesFrame(nil),
		"re":  RetransmissionsFrame(nil),
		"om":  ObserverMetricsFrame(nil),
	} {
		if f == nil || f.Rows() != 0 || len(f.Fields) == 0 {
			t.Errorf("%s: want empty frame with fields", name)
		}
	}
	if f := PacketsPerHourFrame(&corescope.RFAnalytics{}); f.Rows() != 0 || len(f.Fields) != 3 {
		t.Fatal("pph empty")
	}
}
