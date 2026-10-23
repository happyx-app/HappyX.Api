package domain

import "testing"

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
}
