package domain

import (
	"errors"
	"testing"
	"time"
)

const (
	guildA = "100000000000000001"
	userA  = "200000000000000001"
	userB  = "200000000000000002"
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
		{MemberDiscordID: userA, Day: day("2026-10-21"), Mood: MoodNeutral},
	}
	usernames := map[string]string{userA: "ana", userB: "bob"}

	g := Guild{DiscordID: guildA, Settings: DefaultGuildSettings()}
	r := BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)

	if r.Responses != 3 || r.Respondents != 2 {
		t.Fatalf("responses/respondents = %d/%d, want 3/2", r.Responses, r.Respondents)
	}
	if want := (5.0 + 2 + 3) / 3; r.Average != want {
		t.Fatalf("average = %v, want %v", r.Average, want)
	}
	if len(r.Days) != 7 {
		t.Fatalf("days = %d, want 7 (one per day, gaps included)", len(r.Days))
	}
	if r.Days[0].Average != 3.5 || r.Days[1].Responses != 0 {
		t.Fatalf("unexpected day stats: %+v", r.Days[:2])
	}
	if r.Distribution[MoodGreat] != 1 || r.Distribution[MoodAwful] != 0 {
		t.Fatalf("unexpected distribution: %v", r.Distribution)
	}
	if len(r.Messages) != 1 || r.Messages[0].MemberDiscordID != "" || r.Messages[0].Username != "" {
		t.Fatalf("anonymous report leaked identity: %+v", r.Messages)
	}

	g.Settings.Anonymous = false
	r = BuildReport(g, day("2026-10-19"), day("2026-10-25"), entries, usernames)
	if r.Messages[0].Username != "ana" || r.Messages[0].MemberDiscordID != userA {
		t.Fatalf("named report missing identity: %+v", r.Messages)
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
