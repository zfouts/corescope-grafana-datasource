package plugin

// QueryType selects one fixed upstream route. An unknown value is a 400.
type QueryType string

const (
	// Ops — current values only (CoreScope keeps no ops history).
	QueryStats     QueryType = "stats"
	QueryHealth    QueryType = "health"
	QueryEndpoints QueryType = "endpoints"
	QueryMqtt      QueryType = "mqtt"
	// Mesh time series.
	QueryPacketsPerHour  QueryType = "packetsPerHour"
	QueryPayloadTypes    QueryType = "payloadTypes"
	QueryRetransmissions QueryType = "retransmissions"
	QueryObserverMetrics QueryType = "observerMetrics"
	QueryNodeActivity    QueryType = "nodeActivity"
	QueryNodeBattery     QueryType = "nodeBattery"
	// Tables.
	QueryNodes     QueryType = "nodes"
	QueryObservers QueryType = "observers"
)

// QueryModel is the per-panel query JSON. Every field is untrusted (a Viewer
// can POST arbitrary JSON to /api/ds/query). Template variables are already
// interpolated by the frontend; a multi-value variable reaches the backend
// as a comma-joined string, which only Region accepts.
//
// Validation (ParseQuery):
//
//	queryType   one of the constants above, else 400
//	observerId  required for observerMetrics: ^[A-Za-z0-9_-]{1,128}$
//	nodePubkey  required for nodeActivity, nodeBattery: lowercase, then ^[0-9a-f]{64}$
//	region      optional: ^[A-Za-z0-9_-]{1,16}(,[A-Za-z0-9_-]{1,16}){0,15}$; "" or "All"/"$__all" means none
//	role        optional, nodes only: "" | repeater | companion | room | sensor;
//	            "All" / "$__all" mean none
//	resolution  observerMetrics only: "" (auto) | 5m | 1h | 1d
//	limit       nodes only: 0 → 100; clamp 1..500
//	search      nodes only: ≤64 runes, control chars stripped
//
// A failed validation answers that query with backend.StatusBadRequest and a
// fixed message naming the field; it never echoes the value.
type QueryModel struct {
	QueryType  QueryType `json:"queryType"`
	ObserverID string    `json:"observerId,omitempty"`
	NodePubkey string    `json:"nodePubkey,omitempty"`
	Region     string    `json:"region,omitempty"`
	Role       string    `json:"role,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Search     string    `json:"search,omitempty"`
}

// Frame contracts. One frame per query unless noted. Time fields are UTC
// time.Time; nullable upstream values are *float64 / *int64 fields (nil =
// no reading, never 0). Every string from upstream passes through
// SafeString (control chars stripped, ≤256 runes). Frame name = queryType.
//
//	stats            1 row. Fields: totalPackets, totalTransmissions, totalObservations,
//	                 totalNodes, totalNodesAllTime, totalObservers, packetsLastHour,
//	                 packetsLast24h, repeaters, rooms, companions, sensors,
//	                 signatureDrops (int64); storeDataMB, processRSSMB, goHeapInuseMB,
//	                 goSysMB (float64, unit "decmbytes"); version, commit (string).
//	health           1 row. uptime (int64, unit "s"); rssMB, heapUsedMB, heapTotalMB
//	                 (float64, "decmbytes"); eventLoopLagMs, eventLoopP95Ms,
//	                 eventLoopP99Ms, avgRequestMs (float64, "ms"); cacheHitRate
//	                 (float64, "percentunit"); cacheEntries, wsClients,
//	                 wsRejectedDeny, wsRejectedRate, wsRejectedConnCap,
//	                 packetStorePackets, totalRequests, slowQueries (int64); status (string).
//	endpoints        1 row per endpoint, sorted by count desc: endpoint (string),
//	                 count (int64), avgMs, p50Ms, p95Ms, maxMs (float64, "ms").
//	mqtt             1 row per source: name, broker, lastError (string); connected (bool);
//	                 lastConnect, lastDisconnect, lastPacket (*time.Time, nil when 0);
//	                 connectCount, disconnectCount, packetsTotal, packetsLast5m (int64).
//	packetsPerHour   time series, sorted asc: time, packets (int64), avgSnr (*float64;
//	                 joined from signalOverTime on the same hour, nil when absent).
//	payloadTypes     1 row per type, count desc: type (string, the name), count (int64).
//	retransmissions  time series: time, packets, repeaterSum, observers (int64),
//	                 avgRepeaters (float64).
//	observerMetrics  time series: time, noiseFloor (*float64, "dBm"), batteryMv
//	                 (*int64, "mvolt"), txAirtimePct, rxAirtimePct, recvErrorRate
//	                 (*float64, "percent"), packetsSent, packetsRecv (*int64),
//	                 reboot (bool). Frame name = observer name when known.
//	                 Auto resolution: span ≤2d → 5m, ≤30d → 1h, else 1d.
//	nodeActivity     time series: time, packets (int64), certain (*int64).
//	                 days = ceil((now - from) in days), clamp 1..365, because CoreScope
//	                 counts days back from its own clock, not from the range end; rows outside the
//	                 dashboard range are dropped.
//	nodeBattery      time series: time, batteryMv (int64, "mvolt"); same days rule.
//	                 Frame meta Custom: {"status","lowMv","criticalMv"}.
//	nodes            table: publicKey, name, role (string); lastHeard (*time.Time,
//	                 last_heard else last_seen); firstSeen (*time.Time); lat, lon
//	                 (*float64); advertCount, relayCount24h (int64); batteryMv,
//	                 hashSize (*int64); usefulnessScore (float64).
//	observers        table: id, name, iata, clientVersion, radio, nodeRole (string,
//	                 "" when nil); lastSeen, firstSeen (*time.Time); packetCount,
//	                 packetsLastHour (int64); batteryMv (*int64); noiseFloor, lat, lon (*float64).
//
// Time-range rule: for packetsPerHour, payloadTypes and retransmissions the
// dashboard range becomes Window{From, To, Region}.
//
// Resource handler (CallResource): GET "observers" → [{"text": name||id,
// "value": id}] sorted by text; GET "regions" → [{"text": label, "value": code}]
// sorted by value. Any other path or method → 404. req.Path is never forwarded.
