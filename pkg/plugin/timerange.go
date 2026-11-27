package plugin

import (
	"math"
	"time"
)

// ObserverResolution picks the observer-metrics resolution for a dashboard
// range: span <= 48h -> "5m", <= 30 days -> "1h", else "1d".
func ObserverResolution(from, to time.Time) string {
	span := to.Sub(from)
	switch {
	case span <= 48*time.Hour:
		return "5m"
	case span <= 30*24*time.Hour:
		return "1h"
	default:
		return "1d"
	}
}

// DaysForRange is the `days` CoreScope needs to reach back to from: it counts
// back from its own clock, so the span is now - from, not to - from. Ceil in
// days, clamped to 1..365 (from in the future -> 1).
func DaysForRange(now, from time.Time) int {
	days := math.Ceil(now.Sub(from).Hours() / 24)
	if days < 1 || math.IsNaN(days) {
		return 1
	}
	if days > 365 {
		return 365
	}
	return int(days)
}
