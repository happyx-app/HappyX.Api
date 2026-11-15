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
	// Only Ana's public answer names her. The anonymous one and that of Bea, who no longer takes
	// part, are summed up: two people that day, so neither their average nor comments show.
	if len(x.Moods) != 1 || x.Moods[0].Username != "ana" || x.Moods[0].Message != "public" {
		t.Fatalf("named moods: %+v", x.Moods)
	}
	if len(x.AnonymousDays) != 1 || x.AnonymousDays[0].Answers != 2 || !x.AnonymousDays[0].Withheld || x.AnonymousDays[0].Average != 0 || len(x.AnonymousComments) != 0 {
		t.Fatalf("anonymous answers: %+v %v", x.AnonymousDays, x.AnonymousComments)
	}
	// With a third person, the day shows its average and the comments, without day or mood.
	d.Members = append(d.Members, Member{DiscordID: "3", Username: "cris", Active: true})
	d.Moods = append(d.Moods, MoodEntry{MemberDiscordID: "3", Day: day, Mood: 3, Anonymous: true})
	x = BuildGuildExport(d, day)
	if got := x.AnonymousDays[0]; got.Withheld || got.Answers != 3 || got.Average != 3 || len(x.AnonymousComments) != 2 || x.AnonymousComments[0] != "paused" {
		t.Fatalf("anonymous answers of three people: %+v %v", x.AnonymousDays, x.AnonymousComments)
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
	if x := BuildGuildExport(d, day); len(x.Moods) != 0 || x.AnonymousDays[0].Answers != 4 {
		t.Fatalf("anonymous guild names someone: %+v", x.Moods)
	}
}
