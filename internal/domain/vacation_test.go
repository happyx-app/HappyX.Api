package domain

import (
	"strings"
	"testing"
	"time"
)

func TestVacationValidate(t *testing.T) {
	today := day("2026-11-06")
	v := func(from, to string) Vacation {
		return Vacation{UserDiscordID: "200000000000000001", From: day(from), To: day(to)}
	}
	for _, tc := range []struct {
		v    Vacation
		want string // "" when valid
	}{
		{v("2026-11-06", "2026-11-06"), ""},
		{v("2026-11-01", "2026-11-10"), ""}, // already started: fine
		{v("2026-12-20", "2027-01-06"), ""},
		{v("2026-11-01", "2026-11-05"), "already ended"},
		{v("2026-11-10", "2026-11-09"), "before from"},
		{v("2026-11-06", "2027-02-04"), "at most 90 days"}, // 91 days
		{v("2027-11-07", "2027-11-08"), "days ahead"},
		{Vacation{UserDiscordID: "200000000000000001"}, "required"},
		{Vacation{UserDiscordID: "x", From: today, To: today}, "snowflake"},
	} {
		err := tc.v.Validate(today)
		if tc.want == "" && err != nil {
			t.Errorf("%v–%v: unexpected %v", tc.v.From, tc.v.To, err)
		}
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%v–%v: got %v, want %q", tc.v.From, tc.v.To, err, tc.want)
		}
	}
	if err := v("2026-11-06", "2027-02-03").Validate(today); err != nil {
		t.Fatalf("90 days refused: %v", err)
	}
}

func TestStreakSkipsVacations(t *testing.T) {
	g := Guild{DiscordID: "100000000000000001"} // Monday to Friday
	answered := map[time.Time]bool{day("2026-10-26"): true, day("2026-10-27"): true, day("2026-11-05"): true}
	off := Vacations{{From: day("2026-10-28"), To: day("2026-11-04")}}
	if n := Streak(g, answered, off, day("2026-11-05")); n != 3 {
		t.Fatalf("vacation broke the streak: got %d, want 3", n)
	}
	if n := Streak(g, answered, nil, day("2026-11-05")); n != 1 {
		t.Fatalf("without the vacation: got %d, want 1", n)
	}
}
