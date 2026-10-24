package domain

import (
	"testing"
	"time"
)

func TestBuildGuildExport(t *testing.T) {
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC) // Monday
	g := Guild{DiscordID: "100000000000000001", Name: "A", Settings: DefaultGuildSettings()}
	g.Settings.Anonymous = false
	d := GuildData{
		Guild: g,
		Members: []Member{
			{DiscordID: "1", Username: "ana", Active: true},
			{DiscordID: "2", Username: "bea", Active: false},
		},
		Moods: []MoodEntry{
			{MemberDiscordID: "1", Day: day, Mood: 5, Message: "zz", Anonymous: true},
			{MemberDiscordID: "1", Day: day, Mood: 4, Message: "public"},
			{MemberDiscordID: "2", Day: day, Mood: 1, Message: "paused"},
		},
		Rounds:  []GameRound{{Week: day, Question: "?", ProposerUsername: "bea", Proposed: true}},
		Answers: []RoundAnswer{{Week: day, Text: "x", Username: "ana"}},
	}

	x := BuildGuildExport(d, day)
	got := map[string]string{}
	for _, e := range x.Moods {
		got[e.Message] = e.Username
	}
	if got["zz"] != "" || got["public"] != "ana" || got["paused"] != "" {
		t.Fatalf("authors: %v", got)
	}
	if x.Moods[0].Username != "" || x.Moods[2].Username != "ana" {
		t.Fatalf("anonymous answers come first within a day, sorted by mood: %+v", x.Moods)
	}
	if r := x.Rounds[0]; len(r.Answers) != 0 || r.ProposedBy != "" {
		t.Fatalf("answering: %+v", r)
	}
	if r := BuildGuildExport(d, day.AddDate(0, 0, 3)).Rounds[0]; len(r.Answers) != 1 || r.Answers[0].Username != "" || r.ProposedBy != "" {
		t.Fatalf("guessing: %+v", r)
	}
	if r := BuildGuildExport(d, day.AddDate(0, 0, 7)).Rounds[0]; r.Answers[0].Username != "ana" || r.ProposedBy != "bea" {
		t.Fatalf("finished: %+v", r)
	}

	// Back to anonymous hides everyone.
	d.Guild.Settings.Anonymous = true
	for _, e := range BuildGuildExport(d, day).Moods {
		if e.Username != "" {
			t.Fatalf("anonymous guild names %q", e.Username)
		}
	}
}
