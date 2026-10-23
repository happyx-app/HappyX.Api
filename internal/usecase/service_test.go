package usecase

import (
	"testing"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

func TestLastPeriod(t *testing.T) {
	// Thursday 2026-10-22, 23:30 UTC is already Friday in Madrid.
	now := time.Date(2026, 10, 22, 23, 30, 0, 0, time.UTC)
	svc := NewService(nil, nil, nil, nil, func() time.Time { return now })

	g := domain.Guild{Settings: domain.DefaultGuildSettings()}
	from, to := svc.LastPeriod(g)
	if from.Format("2006-01-02") != "2026-10-12" || to.Format("2006-01-02") != "2026-10-18" {
		t.Fatalf("weekly: got %s..%s, want 2026-10-12..2026-10-18", from, to)
	}

	g.Settings.ReportFrequency = domain.ReportMonthly
	from, to = svc.LastPeriod(g)
	if from.Format("2006-01-02") != "2026-09-01" || to.Format("2006-01-02") != "2026-09-30" {
		t.Fatalf("monthly: got %s..%s, want 2026-09-01..2026-09-30", from, to)
	}
}
