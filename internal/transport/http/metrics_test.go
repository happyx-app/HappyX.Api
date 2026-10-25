package http

import (
	"testing"
	"time"
)

func TestMetrics(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 30, 0, 0, time.UTC)
	m := NewMetrics(func() time.Time { return now })
	for i := 1; i <= 100; i++ {
		m.record(time.Duration(i)*time.Millisecond, 200)
	}
	m.record(time.Second, 500)
	if !m.LastBot().IsZero() {
		t.Fatal("no bot call yet")
	}
	m.sawBot()

	hs := m.Hours()
	last := hs[len(hs)-1]
	if len(hs) != MetricsHours || !last.Hour.Equal(now.Truncate(time.Hour)) {
		t.Fatalf("hours: %d, last %v", len(hs), last.Hour)
	}
	if last.Requests != 101 || last.Errors != 1 || last.P50 != 51*time.Millisecond || last.P95 != 96*time.Millisecond {
		t.Fatalf("last hour: %+v", last)
	}
	if !m.LastBot().Equal(now) {
		t.Fatalf("last bot call: %v", m.LastBot())
	}

	// A day later the old hour is gone.
	now = now.Add(MetricsHours * time.Hour)
	m.record(time.Millisecond, 200)
	for _, h := range m.Hours() {
		if h.Requests > 1 {
			t.Fatalf("an hour older than a day is still there: %+v", h)
		}
	}
}
