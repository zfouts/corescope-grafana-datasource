package plugin

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

func unitField[T any](name, unit string, v []T) *data.Field {
	f := data.NewField(name, nil, v)
	if unit != "" {
		f.Config = &data.FieldConfig{Unit: unit}
	}
	return f
}

func tableMeta(f *data.Frame) *data.Frame {
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return f
}

// StatsFrame is the one-row "stats" frame.
func StatsFrame(s *corescope.Stats) *data.Frame {
	var tx int64
	if s.TotalTransmissions != nil {
		tx = int64(*s.TotalTransmissions)
	}
	i := func(name string, v int64) *data.Field { return unitField(name, "", []int64{v}) }
	mb := func(name string, v float64) *data.Field { return unitField(name, "decmbytes", []float64{v}) }
	return data.NewFrame(string(QueryStats),
		i("totalPackets", int64(s.TotalPackets)),
		i("totalTransmissions", tx),
		i("totalObservations", int64(s.TotalObservations)),
		i("totalNodes", int64(s.TotalNodes)),
		i("totalNodesAllTime", int64(s.TotalNodesAllTime)),
		i("totalObservers", int64(s.TotalObservers)),
		i("packetsLastHour", int64(s.PacketsLastHour)),
		i("packetsLast24h", int64(s.PacketsLast24h)),
		i("repeaters", int64(s.Counts.Repeaters)),
		i("rooms", int64(s.Counts.Rooms)),
		i("companions", int64(s.Counts.Companions)),
		i("sensors", int64(s.Counts.Sensors)),
		i("signatureDrops", s.SignatureDrops),
		mb("storeDataMB", s.StoreDataMB),
		mb("processRSSMB", s.ProcessRSSMB),
		mb("goHeapInuseMB", s.GoHeapInuseMB),
		mb("goSysMB", s.GoSysMB),
		data.NewField("version", nil, []string{SafeString(s.Version)}),
		data.NewField("commit", nil, []string{SafeString(s.Commit)}),
	)
}

// HealthFrame is the one-row "health" frame.
func HealthFrame(h *corescope.Health) *data.Frame {
	i := func(name string, v int64) *data.Field { return unitField(name, "", []int64{v}) }
	mb := func(name string, v float64) *data.Field { return unitField(name, "decmbytes", []float64{v}) }
	ms := func(name string, v float64) *data.Field { return unitField(name, "ms", []float64{v}) }
	return data.NewFrame(string(QueryHealth),
		unitField("uptime", "s", []int64{int64(h.Uptime)}),
		mb("rssMB", h.Memory.RSS),
		mb("heapUsedMB", h.Memory.HeapUsed),
		mb("heapTotalMB", h.Memory.HeapTotal),
		ms("eventLoopLagMs", h.EventLoop.CurrentLagMs),
		ms("eventLoopP95Ms", h.EventLoop.P95Ms),
		ms("eventLoopP99Ms", h.EventLoop.P99Ms),
		ms("avgRequestMs", h.Perf.AvgMs),
		unitField("cacheHitRate", "percentunit", []float64{h.Cache.HitRate}),
		i("cacheEntries", int64(h.Cache.Entries)),
		i("wsClients", int64(h.WebSocket.Clients)),
		i("wsRejectedDeny", h.WebSocket.RejectedDeny),
		i("wsRejectedRate", h.WebSocket.RejectedRate),
		i("wsRejectedConnCap", h.WebSocket.RejectedConnCap),
		i("packetStorePackets", int64(h.PacketStore.Packets)),
		i("totalRequests", int64(h.Perf.TotalRequests)),
		i("slowQueries", int64(h.Perf.SlowQueries)),
		data.NewField("status", nil, []string{SafeString(h.Status)}),
	)
}

// EndpointsFrame has one row per endpoint, count desc then endpoint asc.
func EndpointsFrame(p *corescope.Perf) *data.Frame {
	type row struct {
		name string
		st   corescope.EndpointStats
	}
	rows := make([]row, 0, len(p.Endpoints))
	for k, v := range p.Endpoints {
		rows = append(rows, row{SafeString(k), v})
	}
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].st.Count != rows[b].st.Count {
			return rows[a].st.Count > rows[b].st.Count
		}
		return rows[a].name < rows[b].name
	})
	n := len(rows)
	name, count := make([]string, n), make([]int64, n)
	avg, p50, p95, max := make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	for i, r := range rows {
		name[i], count[i] = r.name, int64(r.st.Count)
		avg[i], p50[i], p95[i], max[i] = r.st.AvgMs, r.st.P50Ms, r.st.P95Ms, r.st.MaxMs
	}
	return tableMeta(data.NewFrame(string(QueryEndpoints),
		data.NewField("endpoint", nil, name),
		data.NewField("count", nil, count),
		unitField("avgMs", "ms", avg),
		unitField("p50Ms", "ms", p50),
		unitField("p95Ms", "ms", p95),
		unitField("maxMs", "ms", max),
	))
}

func unixPtr(u int64) *time.Time {
	if u <= 0 {
		return nil
	}
	t := time.Unix(u, 0).UTC()
	return &t
}

// MqttFrame has one row per MQTT source.
func MqttFrame(m *corescope.MqttStatus) *data.Frame {
	n := len(m.Sources)
	name, broker, lastErr := make([]string, n), make([]string, n), make([]string, n)
	conn := make([]bool, n)
	lc, ld, lp := make([]*time.Time, n), make([]*time.Time, n), make([]*time.Time, n)
	cc, dc, pt, p5 := make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n)
	for i, s := range m.Sources {
		name[i], broker[i], lastErr[i] = SafeString(s.Name), SafeString(s.Broker), SafeString(s.LastError)
		conn[i] = s.Connected
		lc[i], ld[i], lp[i] = unixPtr(s.LastConnectUnix), unixPtr(s.LastDisconnectUnix), unixPtr(s.LastPacketUnix)
		cc[i], dc[i], pt[i], p5[i] = s.ConnectCount, s.DisconnectCount, s.PacketsTotal, s.PacketsLast5m
	}
	return tableMeta(data.NewFrame(string(QueryMqtt),
		data.NewField("name", nil, name),
		data.NewField("broker", nil, broker),
		data.NewField("lastError", nil, lastErr),
		data.NewField("connected", nil, conn),
		data.NewField("lastConnect", nil, lc),
		data.NewField("lastDisconnect", nil, ld),
		data.NewField("lastPacket", nil, lp),
		data.NewField("connectCount", nil, cc),
		data.NewField("disconnectCount", nil, dc),
		data.NewField("packetsTotal", nil, pt),
		data.NewField("packetsLast5m", nil, p5),
	))
}
