package plugin

import (
	"testing"
	"time"
)

func TestObserverResolution(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		span time.Duration
		want string
	}{
		{0, "5m"},
		{48 * time.Hour, "5m"},
		{48*time.Hour + time.Second, "1h"},
		{30 * 24 * time.Hour, "1h"},
		{30*24*time.Hour + time.Second, "1d"},
		{400 * 24 * time.Hour, "1d"},
	}
	for _, c := range cases {
		if got := ObserverResolution(from, from.Add(c.span)); got != c.want {
			t.Errorf("span %v: got %s want %s", c.span, got, c.want)
		}
	}
}

func TestDaysForRange(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		back time.Duration // from = now - back
		want int
	}{
		{0, 1},
		{-time.Hour, 1}, // from in the future
		{time.Hour, 1},
		{24 * time.Hour, 1},
		{24*time.Hour + time.Second, 2},
		{7 * 24 * time.Hour, 7},
		{8 * 24 * time.Hour, 8},
		{365 * 24 * time.Hour, 365},
		{400 * 24 * time.Hour, 365},
	}
	for _, c := range cases {
		if got := DaysForRange(now, now.Add(-c.back)); got != c.want {
			t.Errorf("back %v: got %d want %d", c.back, got, c.want)
		}
	}
}
