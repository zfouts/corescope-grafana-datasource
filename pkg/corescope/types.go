// Package corescope is a read-only HTTP client for a CoreScope server's public
// /api/* endpoints, plus the typed response shapes the plugin consumes.
//
// The structs below declare only the fields the plugin reads. CoreScope sends
// more; unknown fields are ignored on decode. Field names and JSON tags mirror
// CoreScope's cmd/server (types.go, db.go, node_battery.go, mqtt_status.go,
// retransmission_pressure.go). Recorded responses live in testdata/.
package corescope

// ─── Ops ──────────────────────────────────────────────────────────────────────

// Healthz is GET /api/healthz. Ready is false until background init finishes.
type Healthz struct {
	Ready bool `json:"ready"`
}

// RoleCounts is the per-role node count in /api/stats and /api/nodes.
type RoleCounts struct {
	Repeaters  int `json:"repeaters"`
	Rooms      int `json:"rooms"`
	Companions int `json:"companions"`
	Sensors    int `json:"sensors"`
}

// Stats is GET /api/stats.
type Stats struct {
	TotalPackets                      int        `json:"totalPackets"`
	TotalTransmissions                *int       `json:"totalTransmissions"`
	TotalObservations                 int        `json:"totalObservations"`
	TotalNodes                        int        `json:"totalNodes"`
	TotalNodesAllTime                 int        `json:"totalNodesAllTime"`
	TotalObservers                    int        `json:"totalObservers"`
	PacketsLastHour                   int        `json:"packetsLastHour"`
	PacketsLast24h                    int        `json:"packetsLast24h"`
	Version                           string     `json:"version"`
	Commit                            string     `json:"commit"`
	Counts                            RoleCounts `json:"counts"`
	SignatureDrops                    int64      `json:"signatureDrops"`
	StoreDataMB                       float64    `json:"storeDataMB"`
	ProcessRSSMB                      float64    `json:"processRSSMB"`
	GoHeapInuseMB                     float64    `json:"goHeapInuseMB"`
	GoSysMB                           float64    `json:"goSysMB"`
	NeighborGraphCacheRebuildFailures uint64     `json:"neighborGraphCacheRebuildFailures"`
}

// CacheStats is the "cache" object of /api/health.
type CacheStats struct {
	Entries    int     `json:"entries"`
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	StaleHits  int     `json:"staleHits"`
	Recomputes int64   `json:"recomputes"`
	HitRate    float64 `json:"hitRate"`
}

// Health is GET /api/health.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Uptime  int    `json:"uptime"` // seconds
	Memory  struct {
		RSS       float64 `json:"rss"` // MB
		HeapUsed  float64 `json:"heapUsed"`
		HeapTotal float64 `json:"heapTotal"`
	} `json:"memory"`
	EventLoop struct {
		CurrentLagMs float64 `json:"currentLagMs"`
		MaxLagMs     float64 `json:"maxLagMs"`
		P95Ms        float64 `json:"p95Ms"`
		P99Ms        float64 `json:"p99Ms"`
	} `json:"eventLoop"`
	Cache     CacheStats `json:"cache"`
	WebSocket struct {
		Clients         int   `json:"clients"`
		RejectedDeny    int64 `json:"rejectedDeny"`
		RejectedRate    int64 `json:"rejectedRate"`
		RejectedConnCap int64 `json:"rejectedConnCap"`
	} `json:"websocket"`
	PacketStore struct {
		Packets     int     `json:"packets"`
		EstimatedMB float64 `json:"estimatedMB"`
	} `json:"packetStore"`
	Perf struct {
		TotalRequests int     `json:"totalRequests"`
		AvgMs         float64 `json:"avgMs"`
		SlowQueries   int     `json:"slowQueries"`
	} `json:"perf"`
}

// EndpointStats is one value of Perf.Endpoints. Percentiles are computed by
// CoreScope since its last restart or /api/perf/reset.
type EndpointStats struct {
	Count int     `json:"count"`
	AvgMs float64 `json:"avgMs"`
	P50Ms float64 `json:"p50Ms"`
	P95Ms float64 `json:"p95Ms"`
	MaxMs float64 `json:"maxMs"`
}

// Perf is GET /api/perf. Endpoints is keyed by route path ("/api/nodes").
type Perf struct {
	Uptime        int                      `json:"uptime"`
	TotalRequests int64                    `json:"totalRequests"`
	AvgMs         float64                  `json:"avgMs"`
	Endpoints     map[string]EndpointStats `json:"endpoints"`
}

// MqttSource is one element of MqttStatus.Sources. Times are unix seconds
// (0 = never). Broker has credentials already redacted by CoreScope.
type MqttSource struct {
	Name               string `json:"name"`
	Broker             string `json:"broker"`
	Connected          bool   `json:"connected"`
	LastConnectUnix    int64  `json:"lastConnectUnix"`
	LastDisconnectUnix int64  `json:"lastDisconnectUnix"`
	LastPacketUnix     int64  `json:"lastPacketUnix"`
	ConnectCount       int64  `json:"connectCount"`
	DisconnectCount    int64  `json:"disconnectCount"`
	PacketsTotal       int64  `json:"packetsTotal"`
	PacketsLast5m      int64  `json:"packetsLast5m"`
	LastError          string `json:"lastError"`
}

// MqttStatus is GET /api/mqtt/status.
type MqttStatus struct {
	Sources              []MqttSource `json:"sources"`
	SampleAt             string       `json:"sampleAt"`
	WatchdogLastTickUnix int64        `json:"watchdogLastTickUnix"`
	WatchdogPanicCount   int64        `json:"watchdogPanicCount"`
}

// ─── Mesh time series ─────────────────────────────────────────────────────────

// HourlyCount: Hour is "2006-01-02T15" (UTC, no minutes, no zone) — parse it
// with ParseTime, never time.RFC3339.
type HourlyCount struct {
	Hour  string `json:"hour"`
	Count int    `json:"count"`
}

// SignalOverTime: Hour has the same "2006-01-02T15" format as HourlyCount.
type SignalOverTime struct {
	Hour   string  `json:"hour"`
	Count  int     `json:"count"`
	AvgSnr float64 `json:"avgSnr"`
}

// PayloadTypeCount is one element of RFAnalytics.PayloadTypes.
type PayloadTypeCount struct {
	Type  int    `json:"type"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// RFAnalytics is GET /api/analytics/rf. Params: region, from, to (RFC3339).
// Returns 503 while CoreScope warms up; the client retries.
type RFAnalytics struct {
	TotalPackets       int                `json:"totalPackets"`
	TotalAllPackets    int                `json:"totalAllPackets"`
	TotalTransmissions int                `json:"totalTransmissions"`
	PacketsPerHour     []HourlyCount      `json:"packetsPerHour"`
	SignalOverTime     []SignalOverTime   `json:"signalOverTime"`
	PayloadTypes       []PayloadTypeCount `json:"payloadTypes"`
}

// RetransmissionBucket: Start is RFC3339.
type RetransmissionBucket struct {
	Start        string  `json:"start"`
	Packets      int     `json:"packets"`
	RepeaterSum  int     `json:"repeater_sum"`
	AvgRepeaters float64 `json:"avg_repeaters"`
	Observers    int     `json:"observers"`
}

// Retransmissions is GET /api/analytics/retransmissions. Params: region,
// from, to (RFC3339). Returns 503 while warming up.
type Retransmissions struct {
	BucketSeconds int                    `json:"bucket_seconds"`
	Buckets       []RetransmissionBucket `json:"buckets"`
}

// MetricsSample is one element of ObserverMetrics.Metrics. Nil = no reading.
type MetricsSample struct {
	Timestamp     string   `json:"timestamp"`
	NoiseFloor    *float64 `json:"noise_floor"`
	BatteryMv     *int     `json:"battery_mv"`
	PacketsSent   *int     `json:"packets_sent"`
	PacketsRecv   *int     `json:"packets_recv"`
	TxAirtimePct  *float64 `json:"tx_airtime_pct"`
	RxAirtimePct  *float64 `json:"rx_airtime_pct"`
	RecvErrorRate *float64 `json:"recv_error_rate"`
	IsReboot      bool     `json:"is_reboot_sample"`
}

// ObserverMetrics is GET /api/observers/{id}/metrics. Params: since, until
// (RFC3339), resolution (5m|1h|1d). Reboots is a list of timestamps.
type ObserverMetrics struct {
	ObserverID   string          `json:"observer_id"`
	ObserverName *string         `json:"observer_name"`
	Reboots      []string        `json:"reboots"`
	Metrics      []MetricsSample `json:"metrics"`
}

// TimeBucket is one element of NodeAnalytics.ActivityTimeline. Bucket is
// RFC3339. Certain is the part of Count that can only be this node.
type TimeBucket struct {
	Bucket  *string `json:"bucket"`
	Label   *string `json:"label"`
	Count   int     `json:"count"`
	Certain *int    `json:"certain"`
}

// NodeAnalytics is GET /api/nodes/{pubkey}/analytics. Param: days (1-365).
type NodeAnalytics struct {
	ActivityTimeline []TimeBucket `json:"activityTimeline"`
}

// BatterySample: Timestamp is RFC3339.
type BatterySample struct {
	Timestamp string `json:"timestamp"`
	BatteryMv int    `json:"battery_mv"`
}

// NodeBattery is GET /api/nodes/{pubkey}/battery. Param: days (1-365).
type NodeBattery struct {
	PublicKey  string          `json:"public_key"`
	Samples    []BatterySample `json:"samples"`
	Status     string          `json:"status"` // ok | low | critical | unknown
	Thresholds struct {
		LowMv      int `json:"low_mv"`
		CriticalMv int `json:"critical_mv"`
	} `json:"thresholds"`
}

// ─── Lists ────────────────────────────────────────────────────────────────────

// Node is one element of NodeList.Nodes.
type Node struct {
	PublicKey       string   `json:"public_key"`
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	LastSeen        *string  `json:"last_seen"`
	LastHeard       *string  `json:"last_heard"` // prefer LastHeard, fall back to LastSeen
	FirstSeen       *string  `json:"first_seen"`
	Lat             *float64 `json:"lat"`
	Lon             *float64 `json:"lon"`
	AdvertCount     int      `json:"advert_count"`
	BatteryMv       *int     `json:"battery_mv"`
	HashSize        *int     `json:"hash_size"`
	RelayCount24h   int      `json:"relay_count_24h"`
	UsefulnessScore float64  `json:"usefulness_score"`
}

// NodeList is GET /api/nodes. Params: limit (server-capped), offset, role,
// region, search, sortBy.
type NodeList struct {
	Nodes   []Node     `json:"nodes"`
	Total   int        `json:"total"`
	Counts  RoleCounts `json:"counts"`
	HasMore bool       `json:"has_more"`
}

// Observer is one element of ObserverList.Observers. ID is the uppercase
// 64-hex pubkey.
type Observer struct {
	ID              string   `json:"id"`
	Name            *string  `json:"name"`
	IATA            *string  `json:"iata"`
	LastSeen        *string  `json:"last_seen"`
	FirstSeen       *string  `json:"first_seen"`
	PacketCount     int      `json:"packet_count"`
	PacketsLastHour int      `json:"packetsLastHour"`
	ClientVersion   *string  `json:"client_version"`
	Radio           *string  `json:"radio"`
	BatteryMv       *int     `json:"battery_mv"`
	NoiseFloor      *float64 `json:"noise_floor"`
	Lat             *float64 `json:"lat"`
	Lon             *float64 `json:"lon"`
	NodeRole        *string  `json:"nodeRole"`
}

// ObserverList is GET /api/observers (no params).
type ObserverList struct {
	Observers  []Observer `json:"observers"`
	ServerTime string     `json:"server_time"`
}

// Regions is GET /api/config/regions: region code → display label
// (e.g. {"OAK":"OAK"}). Codes are what the region query param takes;
// several codes may be joined with ",".
type Regions map[string]string
