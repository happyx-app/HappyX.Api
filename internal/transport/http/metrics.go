package http

import (
	"slices"
	"sync"
	"time"
)

// Metrics keeps, in memory, what the admin dashboard shows about the API itself: latency and
// errors per hour for the last day, and when the bot last called. It starts empty on every
// restart; nothing about who called or what for is kept.
type Metrics struct {
	mu        sync.Mutex
	now       func() time.Time
	started   time.Time
	hours     map[time.Time]*hourStats
	lastBot   time.Time
	sampleCap int
}

type hourStats struct {
	requests int
	errors   int // 5xx
	samples  []time.Duration
}

// HourStats is one hour of requests to the bot and dashboard routes.
type HourStats struct {
	Hour     time.Time
	Requests int
	Errors   int
	P50, P95 time.Duration
}

// MetricsHours is how many hours of latency the admin dashboard gets.
const MetricsHours = 24

func NewMetrics(now func() time.Time) *Metrics {
	if now == nil {
		now = time.Now
	}
	return &Metrics{now: now, started: now(), hours: map[time.Time]*hourStats{}, sampleCap: 4000}
}

func (m *Metrics) record(d time.Duration, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.now().Truncate(time.Hour)
	st := m.hours[h]
	if st == nil {
		st = &hourStats{}
		m.hours[h] = st
		for k := range m.hours {
			if h.Sub(k) >= MetricsHours*time.Hour {
				delete(m.hours, k)
			}
		}
	}
	st.requests++
	if status >= 500 {
		st.errors++
	}
	// Past the cap, keep a spread of the hour instead of only its first minutes.
	if len(st.samples) < m.sampleCap {
		st.samples = append(st.samples, d)
	} else {
		st.samples[st.requests%m.sampleCap] = d
	}
}

func (m *Metrics) sawBot() {
	m.mu.Lock()
	m.lastBot = m.now()
	m.mu.Unlock()
}

// Hours returns the last MetricsHours hours, oldest first, including empty ones.
func (m *Metrics) Hours() []HourStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	end := m.now().Truncate(time.Hour)
	out := make([]HourStats, 0, MetricsHours)
	for i := MetricsHours - 1; i >= 0; i-- {
		h := end.Add(-time.Duration(i) * time.Hour)
		hs := HourStats{Hour: h.UTC()}
		if st := m.hours[h]; st != nil {
			hs.Requests, hs.Errors = st.requests, st.errors
			s := slices.Clone(st.samples)
			slices.Sort(s)
			hs.P50, hs.P95 = percentile(s, 0.50), percentile(s, 0.95)
		}
		out = append(out, hs)
	}
	return out
}

// LastBot is when a request with a valid bot key last arrived; zero since the API started.
func (m *Metrics) LastBot() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastBot
}

func (m *Metrics) Started() time.Time { return m.started }

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p)
	return sorted[i]
}
