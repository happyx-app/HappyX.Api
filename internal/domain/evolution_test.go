package domain

import (
	"testing"
	"time"
)

func TestBuildEvolution(t *testing.T) {
	g := Guild{DiscordID: "100000000000000001"}
	entry := func(d string, m Mood) MoodEntry { return MoodEntry{Day: day(d), Mood: m} }
	entries := []MoodEntry{
		entry("2026-10-10", 1), // before the period: counts for nothing
		entry("2026-10-18", 2),
		entry("2026-10-20", 4),
		entry("2026-10-21", 5),
		entry("2026-10-22", 3),
	}
	from, to := day("2026-10-16"), day("2026-10-22")

	ev := BuildEvolution(g, entries, from, to, day("2026-10-22"))
	if ev.Responses != 4 || ev.Average != 3.5 || len(ev.Entries) != 4 {
		t.Fatalf("period: %+v", ev)
	}
	if ev.Streak != 3 {
		t.Fatalf("streak answered today: got %d, want 3", ev.Streak)
	}
	if ev := BuildEvolution(g, entries, from, to, day("2026-10-23")); ev.Streak != 3 {
		t.Fatalf("not answered yet today keeps the streak: got %d", ev.Streak)
	}
	if ev := BuildEvolution(g, entries, from, to, day("2026-10-24")); ev.Streak != 0 {
		t.Fatalf("a missed day breaks the streak: got %d", ev.Streak)
	}
	if ev := BuildEvolution(g, nil, from, to, day("2026-10-22")); ev.Responses != 0 || ev.Average != 0 || ev.Streak != 0 {
		t.Fatalf("no answers: %+v", ev)
	}

	// Weekends are off by default: answering Friday and Monday keeps the streak going.
	weekend := []MoodEntry{entry("2026-10-22", 3), entry("2026-10-23", 3), entry("2026-10-26", 4)}
	if ev := BuildEvolution(g, weekend, from, to, day("2026-10-26")); ev.Streak != 3 {
		t.Fatalf("weekend breaks the streak: got %d, want 3", ev.Streak)
	}
	// An answer given on a day off still counts.
	weekend = append(weekend, entry("2026-10-25", 4))
	if ev := BuildEvolution(g, weekend, from, to, day("2026-10-26")); ev.Streak != 4 {
		t.Fatalf("answer on a day off: got %d, want 4", ev.Streak)
	}
	// When Saturday is a working day, missing it breaks the streak.
	g.Settings.WorkingDays = MondayToFriday | 1<<time.Saturday
	if ev := BuildEvolution(g, weekend, from, to, day("2026-10-26")); ev.Streak != 2 {
		t.Fatalf("missed working Saturday: got %d, want 2", ev.Streak)
	}
}
