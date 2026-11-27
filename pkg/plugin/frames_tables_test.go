package plugin

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

func TestNodesFrame(t *testing.T) {
	var l corescope.NodeList
	loadOpsFixture(t, "nodes.json", &l)
	f := NodesFrame(&l)
	if f.Name != "nodes" || f.Rows() != len(l.Nodes) || f.Rows() != 5 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeTable {
		t.Errorf("meta = %+v", f.Meta)
	}
	s, ts, nf, i, ni := data.FieldTypeString, data.FieldTypeNullableTime, data.FieldTypeNullableFloat64, data.FieldTypeInt64, data.FieldTypeNullableInt64
	checkOpsFields(t, f, []opsFieldSpec{
		{"publicKey", s, ""}, {"name", s, ""}, {"role", s, ""},
		{"lastHeard", ts, ""}, {"firstSeen", ts, ""}, {"lat", nf, ""}, {"lon", nf, ""},
		{"advertCount", i, ""}, {"relayCount24h", i, ""},
		{"batteryMv", ni, ""}, {"hashSize", ni, ""}, {"usefulnessScore", data.FieldTypeFloat64, ""},
	})
	// Upstream order kept.
	if v := opsField(t, f, "name").At(0); v != "KN6PLV-BrkOxfLA-Yebes" {
		t.Errorf("name[0] = %v", v)
	}
	if v := opsField(t, f, "advertCount").At(0); v != int64(61) {
		t.Errorf("advertCount[0] = %v", v)
	}
	if v := opsField(t, f, "lastHeard").At(0).(*time.Time); v == nil || !v.Equal(time.Date(2026, 11, 24, 1, 48, 19, 0, time.UTC)) {
		t.Errorf("lastHeard[0] = %v", v)
	}
	if v := opsField(t, f, "firstSeen").At(0).(*time.Time); v == nil || v.Year() != 2026 || v.Month() != 3 {
		t.Errorf("firstSeen[0] = %v", v)
	}
	if v := opsField(t, f, "batteryMv").At(0).(*int64); v != nil {
		t.Errorf("batteryMv[0] = %v, want nil", *v)
	}
	if v := opsField(t, f, "hashSize").At(0).(*int64); v == nil || *v != 1 {
		t.Errorf("hashSize[0] = %v", v)
	}
	if v := opsField(t, f, "lat").At(0).(*float64); v == nil || *v != 37.888 {
		t.Errorf("lat[0] = %v", v)
	}
}

func TestNodesFrameHostileName(t *testing.T) {
	var l corescope.NodeList
	loadOpsFixture(t, "nodes.json", &l)
	got := opsField(t, NodesFrame(&l), "name").At(1).(string)
	if strings.ContainsRune(got, '\x07') {
		t.Fatalf("BEL not stripped: %q", got)
	}
	// Everything else is plain text; escaping is Grafana's job.
	if want := `<img src=x onerror=alert(1)>=HYPERLINK("x")`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNodesFrameLastHeardFallbackAndBadTimes(t *testing.T) {
	seen, bad := "2026-11-01T00:00:00Z", "garbage"
	l := &corescope.NodeList{Nodes: []corescope.Node{
		{PublicKey: "a", LastSeen: &seen},                  // no last_heard → last_seen
		{PublicKey: "b", LastHeard: &bad, LastSeen: &seen}, // unparseable last_heard → last_seen
		{PublicKey: "c", LastHeard: &bad},                  // nothing parseable → nil
		{PublicKey: "d"},
	}}
	f := NodesFrame(l)
	lh := opsField(t, f, "lastHeard")
	for i, wantSet := range []bool{true, true, false, false} {
		v := lh.At(i).(*time.Time)
		if (v != nil) != wantSet {
			t.Errorf("row %d lastHeard = %v, want set=%v", i, v, wantSet)
		}
	}
	if v := opsField(t, f, "firstSeen").At(0).(*time.Time); v != nil {
		t.Errorf("firstSeen = %v", v)
	}
}

func TestObserversFrame(t *testing.T) {
	var l corescope.ObserverList
	loadOpsFixture(t, "observers.json", &l)
	f := ObserversFrame(&l)
	if f.Name != "observers" || f.Rows() != 3 {
		t.Fatalf("name=%s rows=%d", f.Name, f.Rows())
	}
	if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeTable {
		t.Errorf("meta = %+v", f.Meta)
	}
	s, ts, nf, i := data.FieldTypeString, data.FieldTypeNullableTime, data.FieldTypeNullableFloat64, data.FieldTypeInt64
	checkOpsFields(t, f, []opsFieldSpec{
		{"id", s, ""}, {"name", s, ""}, {"iata", s, ""}, {"clientVersion", s, ""}, {"radio", s, ""}, {"nodeRole", s, ""},
		{"lastSeen", ts, ""}, {"firstSeen", ts, ""}, {"packetCount", i, ""}, {"packetsLastHour", i, ""},
		{"batteryMv", data.FieldTypeNullableInt64, ""}, {"noiseFloor", nf, ""}, {"lat", nf, ""}, {"lon", nf, ""},
	})
	if v := opsField(t, f, "id").At(0); v != "D5EF2ED77CEABCB22082F8794D5F9D89F2257E70533863E924DEBE6EBDDA312A" {
		t.Errorf("id[0] = %v", v)
	}
	if v := opsField(t, f, "name").At(0); v != "Kennedy Repeater" {
		t.Errorf("name[0] = %v", v)
	}
	if v := opsField(t, f, "iata").At(0); v != "OAK" {
		t.Errorf("iata[0] = %v", v)
	}
	if v := opsField(t, f, "radio").At(0); v != "910.5250244,62.5,7,5" {
		t.Errorf("radio[0] = %v", v)
	}
	if v := opsField(t, f, "packetCount").At(0); v != int64(20771) {
		t.Errorf("packetCount[0] = %v", v)
	}
	if v := opsField(t, f, "noiseFloor").At(0).(*float64); v != nil {
		t.Errorf("noiseFloor[0] = %v, want nil", *v)
	}
	if v := opsField(t, f, "lastSeen").At(0).(*time.Time); v == nil || v.Day() != 24 {
		t.Errorf("lastSeen[0] = %v", v)
	}
}

func TestObserversFrameNilStrings(t *testing.T) {
	bad := "nope"
	f := ObserversFrame(&corescope.ObserverList{Observers: []corescope.Observer{{ID: "X", LastSeen: &bad}}})
	for _, n := range []string{"name", "iata", "clientVersion", "radio", "nodeRole"} {
		if v := opsField(t, f, n).At(0); v != "" {
			t.Errorf("%s = %v, want empty", n, v)
		}
	}
	if v := opsField(t, f, "lastSeen").At(0).(*time.Time); v != nil {
		t.Errorf("lastSeen = %v, want nil", v)
	}
}
