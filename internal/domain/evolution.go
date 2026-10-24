package domain

import "time"

// Evolution is one person's own answers in one guild over a period, for their "my evolution"
// page. It only ever holds the viewer's own entries, so anonymity does not apply.
type Evolution struct {
	Guild     Guild
	Entries   []MoodEntry // in the period, oldest first
	Responses int
	Average   float64 // 0 when there are no responses
	// Streak is how many days in a row up to today the person has answered. Not having
	// answered yet today doesn't break it, and neither do the guild's non-working days.
	Streak int
}

// BuildEvolution summarises entries (the person's answers in g, oldest first, which may
// reach before from so the streak can be counted) for the period [from, to].
func BuildEvolution(g Guild, entries []MoodEntry, from, to, today time.Time) Evolution {
	ev := Evolution{Guild: g}
	answered := map[time.Time]bool{}
	sum := 0
	for _, e := range entries {
		d := TruncateDay(e.Day)
		answered[d] = true
		if d.Before(from) || d.After(to) {
			continue
		}
		ev.Entries = append(ev.Entries, e)
		sum += int(e.Mood)
	}
	ev.Responses = len(ev.Entries)
	if ev.Responses > 0 {
		ev.Average = float64(sum) / float64(ev.Responses)
	}
	ev.Streak = Streak(g, answered, today)
	return ev
}

// Streak counts how many days in a row up to today someone answered in g, given the days they
// answered (UTC midnights). Not having answered yet today doesn't break it, and neither do the
// guild's non-working days.
func Streak(g Guild, answered map[time.Time]bool, today time.Time) int {
	n := 0
	d := TruncateDay(today)
	if !answered[d] {
		d = d.AddDate(0, 0, -1)
	}
	for ; answered[d] || !g.Settings.IsWorkingDay(d); d = d.AddDate(0, 0, -1) {
		if answered[d] {
			n++
		}
	}
	return n
}
