package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	guildA = "100000000000000001"
	userA  = "200000000000000001"
	userB  = "200000000000000002"
	userC  = "200000000000000003"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestMoodEntryValidate(t *testing.T) {
	ok := MoodEntry{GuildDiscordID: guildA, MemberDiscordID: userA, Day: day("2026-10-20"), Mood: MoodGood}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	cases := map[string]func(*MoodEntry){
		"mood zero":       func(e *MoodEntry) { e.Mood = 0 },
		"mood six":        func(e *MoodEntry) { e.Mood = 6 },
		"bad guild id":    func(e *MoodEntry) { e.GuildDiscordID = "abc" },
		"message too big": func(e *MoodEntry) { e.Message = string(make([]rune, MaxMessageLength+1)) },
		"missing day":     func(e *MoodEntry) { e.Day = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := ok
			mutate(&e)
			if err := e.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestGuildSettingsValidate(t *testing.T) {
	s := DefaultGuildSettings()
	if err := s.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	s.Timezone = "Mars/Olympus"
	if err := s.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad timezone, got %v", err)
	}
}

func TestBuildReport(t *testing.T) {
	entries := []MoodEntry{
		{MemberDiscordID: userA, Day: day("2026-10-19"), Mood: MoodGreat, Message: "great day"},
		{MemberDiscordID: userB, Day: day("2026-10-19"), Mood: MoodBad},
		{MemberDiscordID: userC, Day: day("2026-10-19"), Mood: MoodGood},
		{MemberDiscordID: userA, Day: day("2026-10-21"), Mood: MoodNeutral},
	}
	usernames := map[string]string{userA: "ana", userB: "bob", userC: "cris"}

	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)

	if r.Responses != 4 || r.Respondents != 3 || r.Withheld || r.AnonymityThreshold != 3 {
		t.Fatalf("responses/respondents/withheld = %d/%d/%v, want 4/3/false", r.Responses, r.Respondents, r.Withheld)
	}
	if want := (5.0 + 2 + 4 + 3) / 4; r.Average != want {
		t.Fatalf("average = %v, want %v", r.Average, want)
	}
	if len(r.Days) != 5 {
		t.Fatalf("days = %d, want 5 (one per working day, gaps included)", len(r.Days))
	}
	if r.Days[0].Average != 11.0/3 || r.Days[1].Responses != 0 {
		t.Fatalf("unexpected day stats: %+v", r.Days[:2])
	}
	// Wednesday only one person answered: its average would be theirs.
	if d := r.Days[2]; d.Responses != 1 || !d.Withheld || d.Average != 0 {
		t.Fatalf("a day with one anonymous answer keeps its average: %+v", d)
	}
	if r.Distribution[MoodGreat] != 1 || r.Distribution[MoodAwful] != 0 {
		t.Fatalf("unexpected distribution: %v", r.Distribution)
	}
	if len(r.Messages) != 1 || r.Messages[0].MemberDiscordID != "" || r.Messages[0].Username != "" {
		t.Fatalf("anonymous report leaked identity: %+v", r.Messages)
	}
	if m := r.Messages[0]; !m.Day.IsZero() || m.Mood != 0 || !m.Anonymous() {
		t.Fatalf("an anonymous comment keeps its day or mood: %+v", m)
	}

	g.Settings.Anonymous = false
	r = BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)
	if m := r.Messages[0]; m.Username != "ana" || m.MemberDiscordID != userA || m.Day != day("2026-10-19") || m.Mood != MoodGreat {
		t.Fatalf("named report missing identity: %+v", r.Messages)
	}
	if d := r.Days[2]; d.Withheld || d.Average != 3 {
		t.Fatalf("a named day hides its average: %+v", d)
	}
}

func TestBuildReportWithheldBelowThreshold(t *testing.T) {
	entries := []MoodEntry{
		{MemberDiscordID: userA, Day: day("2026-10-19"), Mood: MoodGreat, Message: "great day"},
		{MemberDiscordID: userB, Day: day("2026-10-19"), Mood: MoodBad, Message: "rough"},
		{MemberDiscordID: userA, Day: day("2026-10-20"), Mood: MoodGood},
	}
	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil)
	if !r.Withheld || r.Responses != 3 || r.Respondents != 2 {
		t.Fatalf("two people of a threshold of 3: %+v", r)
	}
	if r.Average != 0 || len(r.Messages) != 0 || r.Distribution[MoodGreat] != 0 {
		t.Fatalf("a withheld report shows moods or comments: %+v", r)
	}
	for _, d := range r.Days {
		if d.Average != 0 || (d.Responses > 0 && !d.Withheld) {
			t.Fatalf("a withheld report shows a day's average: %+v", d)
		}
	}

	// A higher threshold withholds bigger groups too.
	entries = append(entries, MoodEntry{MemberDiscordID: userC, Day: day("2026-10-19"), Mood: MoodGood})
	if r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil); r.Withheld {
		t.Fatal("three people reach the default threshold")
	}
	g.Settings.AnonymityThreshold = 5
	if r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil); !r.Withheld || r.AnonymityThreshold != 5 {
		t.Fatalf("threshold 5 with three people: %+v", r)
	}

	if r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), nil, nil); r.Withheld {
		t.Fatal("a week without answers has nothing to withhold")
	}

	// A named team sees everything, however small.
	g.Settings.Anonymous = false
	if r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries[:2], nil); r.Withheld || r.Average == 0 {
		t.Fatalf("named report withheld: %+v", r)
	}
}

func TestBuildReportShufflesAnonymousComments(t *testing.T) {
	var entries []MoodEntry
	people := []string{userA, userB, userC}
	for i, text := range []string{"lunes", "martes", "miércoles", "jueves", "viernes", "otro lunes"} {
		entries = append(entries, MoodEntry{MemberDiscordID: people[i%3], Day: day("2026-10-19").AddDate(0, 0, i%5), Mood: MoodGood, Message: text})
	}
	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	text := func(r Report) string {
		var out []string
		for _, m := range r.Messages {
			out = append(out, m.Message)
		}
		return strings.Join(out, ",")
	}
	first := text(BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil))
	if first == "lunes,martes,miércoles,jueves,viernes,otro lunes" {
		t.Fatal("anonymous comments come in the order they were written")
	}
	slices.Reverse(entries)
	if again := text(BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil)); again != first {
		t.Fatalf("the order changes between readings: %q vs %q", first, again)
	}
}

func TestAnonymousOn(t *testing.T) {
	s := DefaultGuildSettings()
	if !s.AnonymousOn(day("2026-10-20")) {
		t.Fatal("an anonymous guild")
	}
	s.Anonymous, s.NamedSince = false, day("2026-10-21")
	if !s.AnonymousOn(day("2026-10-20")) || s.AnonymousOn(day("2026-10-21")) {
		t.Fatal("names start on NamedSince")
	}
	s.AnonymityThreshold = 2
	if err := s.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("threshold 2: want ErrInvalid, got %v", err)
	}
}

func TestBuildReportShowsNonWorkingDaysOnlyWithAnswers(t *testing.T) {
	entries := []MoodEntry{
		{MemberDiscordID: userA, Day: day("2026-10-24"), Mood: MoodGood}, // Saturday
	}
	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil)
	if len(r.Days) != 6 || r.Days[5].Day != day("2026-10-24") || r.Days[5].Responses != 1 {
		t.Fatalf("want Monday-Friday plus the answered Saturday, got %+v", r.Days)
	}

	g.Settings.WorkingDays = allWeekdays
	if r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, nil); len(r.Days) != 7 {
		t.Fatalf("every day working: got %d days", len(r.Days))
	}
}

func TestWeekdays(t *testing.T) {
	w, err := WeekdaysOf([]int{7, 1, 3})
	if err != nil {
		t.Fatal(err)
	}
	if !w.Has(time.Sunday) || !w.Has(time.Monday) || w.Has(time.Tuesday) {
		t.Fatalf("unexpected set %07b", w)
	}
	if got := w.ISO(); len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 7 {
		t.Fatalf("ISO = %v, want [1 3 7]", got)
	}
	if got := MondayToFriday.ISO(); len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Fatalf("Monday to Friday = %v", got)
	}
	for _, bad := range [][]int{{0}, {8}, {1, -1}} {
		if _, err := WeekdaysOf(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: want ErrInvalid, got %v", bad, err)
		}
	}

	s := DefaultGuildSettings()
	s.WorkingDays = 0
	if err := s.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no working days: want ErrInvalid, got %v", err)
	}
	if !s.IsWorkingDay(day("2026-10-23")) || s.IsWorkingDay(day("2026-10-24")) {
		t.Fatal("unset working days should count as Monday to Friday")
	}
}

func TestBuildReportKeepsEarlierAnswersAnonymous(t *testing.T) {
	entries := []MoodEntry{
		{MemberDiscordID: userA, Day: day("2026-10-19"), Mood: MoodGreat, Message: "said in private", Anonymous: true},
		{MemberDiscordID: userC, Day: day("2026-10-19"), Mood: MoodGood, Anonymous: true},
		{MemberDiscordID: userB, Day: day("2026-10-20"), Mood: MoodBad, Message: "said in public"},
	}
	usernames := map[string]string{userA: "ana", userB: "bob", userC: "cris"}
	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	g.Settings.Anonymous = false

	r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)
	if len(r.Messages) != 2 {
		t.Fatalf("messages: %+v", r.Messages)
	}
	if m := r.Messages[0]; m.Username != "bob" || m.MemberDiscordID != userB {
		t.Fatalf("answer given in public lost its author: %+v", m)
	}
	if m := r.Messages[1]; m.MemberDiscordID != "" || m.Username != "" || !m.Day.IsZero() {
		t.Fatalf("answer given while anonymous shows its author or day: %+v", m)
	}
	// Only two people answered while anonymous: the earlier day keeps its average hidden.
	if d := r.Days[0]; !d.Withheld {
		t.Fatalf("anonymous answers of two people show their average: %+v", d)
	}

	g.Settings.Anonymous = true
	r = BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)
	for _, m := range r.Messages {
		if m.MemberDiscordID != "" || m.Username != "" {
			t.Fatalf("anonymous guild shows an author: %+v", m)
		}
	}
}

func TestLanguageValidation(t *testing.T) {
	for _, ok := range []string{"en", "es", "ca", "pt-BR", "ast"} {
		s := DefaultGuildSettings()
		s.Language = ok
		if err := s.Validate(); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "e", "EN", "en-us", "en_US", "english", "e1"} {
		s := DefaultGuildSettings()
		s.Language = bad
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted", bad)
		}
	}
}
