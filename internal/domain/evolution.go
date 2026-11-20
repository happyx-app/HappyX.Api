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
	// answered yet today doesn't break it, and neither do the guild's non-working days or the
	// person's vacations.
	Streak int
	// AnonymousToday is whether today's answer is anonymous (see GuildSettings.AnonymousOn).
	AnonymousToday bool
	// ThresholdToday is the anonymity threshold today's answer is given under (see
	// GuildSettings.ThresholdOn).
	ThresholdToday int
	// SurveyDays are the days of the period the survey went out in the guild since the person
	// joined it, oldest first: a day among them without an answer is one they didn't answer,
	// unless their vacation covers it.
	SurveyDays []time.Time
	// Vacations are the person's days off that touch the period and apply in the guild: their
	// own and the ones set there, past ones included.
	Vacations Vacations
	// Events are the guild's team events that touch the period, by start date.
	Events []TeamEvent
}

// BuildEvolution summarises entries (the person's answers in g, oldest first, which may
// reach before from so the streak can be counted) for the period [from, to]. off are the
// person's vacations in g.
func BuildEvolution(g Guild, entries []MoodEntry, off Vacations, from, to, today time.Time) Evolution {
	ev := Evolution{Guild: g, AnonymousToday: g.Settings.AnonymousOn(today), ThresholdToday: g.Settings.ThresholdOn(today)}
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
	ev.Streak = Streak(g, answered, off, today)
	return ev
}

// Streak counts how many days in a row up to today someone answered in g, given the days they
// answered (UTC midnights). Not having answered yet today doesn't break it, and neither do the
// guild's non-working days or the days off their vacations cover.
func Streak(g Guild, answered map[time.Time]bool, off Vacations, today time.Time) int {
	n := 0
	d := TruncateDay(today)
	if !answered[d] {
		d = d.AddDate(0, 0, -1)
	}
	for ; answered[d] || !g.Settings.IsWorkingDay(d) || off.Covers(d); d = d.AddDate(0, 0, -1) {
		if answered[d] {
			n++
		}
	}
	return n
}
