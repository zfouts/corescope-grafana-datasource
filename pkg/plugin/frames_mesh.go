package plugin

import (
	"fmt"
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

func meshUnit(f *data.Field, u string) *data.Field {
	f.Config = &data.FieldConfig{Unit: u}
	return f
}

// PacketsPerHourFrame: time, packets, avgSnr (nil when no signal row for the hour).
func PacketsPerHourFrame(rf *corescope.RFAnalytics) *data.Frame {
	type row struct {
		t time.Time
		n int64
	}
	var rows []row
	snr := map[time.Time]float64{}
	if rf != nil {
		for _, s := range rf.SignalOverTime {
			if t, ok := corescope.ParseTime(s.Hour); ok {
				snr[t] = s.AvgSnr
			}
		}
		for _, h := range rf.PacketsPerHour {
			if t, ok := corescope.ParseTime(h.Hour); ok {
				rows = append(rows, row{t, int64(h.Count)})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].t.Before(rows[j].t) })
	times := make([]time.Time, len(rows))
	pk := make([]int64, len(rows))
	avg := make([]*float64, len(rows))
	for i, r := range rows {
		times[i], pk[i] = r.t, r.n
		if v, ok := snr[r.t]; ok {
			v := v
			avg[i] = &v
		}
	}
	return data.NewFrame(string(QueryPacketsPerHour),
		data.NewField("time", nil, times),
		data.NewField("packets", nil, pk),
		data.NewField("avgSnr", nil, avg),
	)
}

// PayloadTypesFrame: type, count; count desc then name asc.
func PayloadTypesFrame(rf *corescope.RFAnalytics) *data.Frame {
	type row struct {
		name string
		n    int64
	}
	var rows []row
	if rf != nil {
		for _, p := range rf.PayloadTypes {
			name := SafeString(p.Name)
			if name == "" {
				name = fmt.Sprintf("type %d", p.Type)
			}
			rows = append(rows, row{name, int64(p.Count)})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].name < rows[j].name
	})
	names := make([]string, len(rows))
	counts := make([]int64, len(rows))
	for i, r := range rows {
		names[i], counts[i] = r.name, r.n
	}
	return data.NewFrame(string(QueryPayloadTypes),
		data.NewField("type", nil, names),
		data.NewField("count", nil, counts),
	)
}

// RetransmissionsFrame: time, packets, repeaterSum, observers, avgRepeaters.
func RetransmissionsFrame(r *corescope.Retransmissions) *data.Frame {
	var bs []corescope.RetransmissionBucket
	var ts []time.Time
	if r != nil {
		for _, b := range r.Buckets {
			if t, ok := corescope.ParseTime(b.Start); ok {
				bs = append(bs, b)
				ts = append(ts, t)
			}
		}
	}
	idx := make([]int, len(bs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return ts[idx[a]].Before(ts[idx[b]]) })
	times := make([]time.Time, len(bs))
	pk := make([]int64, len(bs))
	rs := make([]int64, len(bs))
	ob := make([]int64, len(bs))
	avg := make([]float64, len(bs))
	for o, i := range idx {
		times[o] = ts[i]
		pk[o] = int64(bs[i].Packets)
		rs[o] = int64(bs[i].RepeaterSum)
		ob[o] = int64(bs[i].Observers)
		avg[o] = bs[i].AvgRepeaters
	}
	return data.NewFrame(string(QueryRetransmissions),
		data.NewField("time", nil, times),
		data.NewField("packets", nil, pk),
		data.NewField("repeaterSum", nil, rs),
		data.NewField("observers", nil, ob),
		data.NewField("avgRepeaters", nil, avg),
	)
}

// ObserverMetricsFrame: one row per sample; nil readings stay nil.
func ObserverMetricsFrame(m *corescope.ObserverMetrics) *data.Frame {
	name := string(QueryObserverMetrics)
	var samples []corescope.MetricsSample
	var ts []time.Time
	if m != nil {
		if n := SafeStringPtr(m.ObserverName); n != "" {
			name = n
		} else if id := SafeString(m.ObserverID); id != "" {
			name = id
		}
		for _, s := range m.Metrics {
			if t, ok := corescope.ParseTime(s.Timestamp); ok {
				samples = append(samples, s)
				ts = append(ts, t)
			}
		}
	}
	n := len(samples)
	nf := make([]*float64, n)
	bat := make([]*int64, n)
	tx := make([]*float64, n)
	rx := make([]*float64, n)
	er := make([]*float64, n)
	sent := make([]*int64, n)
	recv := make([]*int64, n)
	reboot := make([]bool, n)
	for i, s := range samples {
		nf[i], bat[i] = s.NoiseFloor, intPtr64(s.BatteryMv)
		tx[i], rx[i], er[i] = s.TxAirtimePct, s.RxAirtimePct, s.RecvErrorRate
		sent[i], recv[i] = intPtr64(s.PacketsSent), intPtr64(s.PacketsRecv)
		reboot[i] = s.IsReboot
	}
	times := make([]time.Time, n)
	copy(times, ts)
	return data.NewFrame(name,
		data.NewField("time", nil, times),
		meshUnit(data.NewField("noiseFloor", nil, nf), "dBm"),
		meshUnit(data.NewField("batteryMv", nil, bat), "mvolt"),
		meshUnit(data.NewField("txAirtimePct", nil, tx), "percent"),
		meshUnit(data.NewField("rxAirtimePct", nil, rx), "percent"),
		meshUnit(data.NewField("recvErrorRate", nil, er), "percent"),
		data.NewField("packetsSent", nil, sent),
		data.NewField("packetsRecv", nil, recv),
		data.NewField("reboot", nil, reboot),
	)
}

func meshInRange(t, from, to time.Time) bool { return !t.Before(from) && !t.After(to) }

// NodeActivityFrame: time, packets, certain; rows outside [from, to] dropped.
func NodeActivityFrame(a *corescope.NodeAnalytics, from, to time.Time) *data.Frame {
	times := []time.Time{}
	pk := []int64{}
	cert := []*int64{}
	if a != nil {
		type row struct {
			t time.Time
			n int64
			c *int64
		}
		var rows []row
		for _, b := range a.ActivityTimeline {
			var s string
			switch {
			case b.Bucket != nil:
				s = *b.Bucket
			case b.Label != nil:
				s = *b.Label
			default:
				continue
			}
			t, ok := corescope.ParseTime(s)
			// Buckets are stamped at the start of the hour: keep the partial
			// first hour whose bucket start precedes from.
			if !ok || !t.Add(time.Hour).After(from) || t.After(to) {
				continue
			}
			rows = append(rows, row{t, int64(b.Count), intPtr64(b.Certain)})
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].t.Before(rows[j].t) })
		for _, r := range rows {
			times = append(times, r.t)
			pk = append(pk, r.n)
			cert = append(cert, r.c)
		}
	}
	return data.NewFrame(string(QueryNodeActivity),
		data.NewField("time", nil, times),
		data.NewField("packets", nil, pk),
		data.NewField("certain", nil, cert),
	)
}

// NodeBatteryFrame: time, batteryMv; rows outside [from, to] dropped.
func NodeBatteryFrame(b *corescope.NodeBattery, from, to time.Time) *data.Frame {
	times := []time.Time{}
	mv := []int64{}
	custom := map[string]any{"status": "", "lowMv": int64(0), "criticalMv": int64(0)}
	if b != nil {
		type row struct {
			t time.Time
			v int64
		}
		var rows []row
		for _, s := range b.Samples {
			t, ok := corescope.ParseTime(s.Timestamp)
			if !ok || !meshInRange(t, from, to) {
				continue
			}
			rows = append(rows, row{t, int64(s.BatteryMv)})
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].t.Before(rows[j].t) })
		for _, r := range rows {
			times = append(times, r.t)
			mv = append(mv, r.v)
		}
		custom = map[string]any{
			"status":     SafeString(b.Status),
			"lowMv":      int64(b.Thresholds.LowMv),
			"criticalMv": int64(b.Thresholds.CriticalMv),
		}
	}
	f := data.NewFrame(string(QueryNodeBattery),
		data.NewField("time", nil, times),
		meshUnit(data.NewField("batteryMv", nil, mv), "mvolt"),
	)
	f.Meta = &data.FrameMeta{Custom: custom}
	return f
}
