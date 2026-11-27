package corescope

import (
	"context"
	"time"
)

// API is the read-only surface the plugin uses. *Client implements it; plugin
// tests substitute a fake. Every method is one GET; none writes.
type API interface {
	Healthz(ctx context.Context) (*Healthz, error)
	Stats(ctx context.Context) (*Stats, error)
	Health(ctx context.Context) (*Health, error)
	Perf(ctx context.Context) (*Perf, error)
	MqttStatus(ctx context.Context) (*MqttStatus, error)

	// RF and Retransmissions send from/to as RFC3339 UTC, and region when
	// non-empty.
	RF(ctx context.Context, w Window) (*RFAnalytics, error)
	Retransmissions(ctx context.Context, w Window) (*Retransmissions, error)

	// ObserverMetrics sends since/until as RFC3339 UTC and resolution.
	ObserverMetrics(ctx context.Context, observerID string, from, to time.Time, resolution string) (*ObserverMetrics, error)

	// NodeAnalytics and NodeBattery send days (1-365).
	NodeAnalytics(ctx context.Context, pubkey string, days int) (*NodeAnalytics, error)
	NodeBattery(ctx context.Context, pubkey string, days int) (*NodeBattery, error)

	Nodes(ctx context.Context, q NodeQuery) (*NodeList, error)
	Observers(ctx context.Context) (*ObserverList, error)
	Regions(ctx context.Context) (Regions, error)
}

// Window is a time range plus optional region filter.
type Window struct {
	From, To time.Time
	Region   string
}

// NodeQuery filters GET /api/nodes. Zero values are omitted from the URL.
type NodeQuery struct {
	Limit  int
	Role   string
	Region string
	Search string
}

// Error is a non-2xx response. Message is CoreScope's {"error": "..."} text,
// truncated, never the raw body.
type Error struct {
	Status  int
	Path    string
	Message string
}

func (e *Error) Error() string {
	return "corescope: " + e.Path + ": " + itoa(e.Status) + " " + e.Message
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ParseTime parses every timestamp form CoreScope emits, always as UTC:
// RFC3339 / RFC3339Nano ("2026-11-24T01:48:19Z", "...19.081Z"),
// "2006-01-02T15" (RF hour keys), "2006-01-02 15:04:05" (SQLite datetime).
// Returns ok=false for anything else; callers skip that row.
func ParseTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
