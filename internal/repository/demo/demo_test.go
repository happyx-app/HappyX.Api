package demo_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/repository/demo"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

func at(s string) func() time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return t }
}

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestDemoReport(t *testing.T) {
	ctx := context.Background()
	svc := demo.NewStore("es", at("2026-10-31T10:00:00Z")).NewService()
	r, err := svc.Report(ctx, demo.GuildID, day("2026-05-01"), day("2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Anonymous || r.Respondents < 10 || r.Responses < 1000 || r.Average < 3 || r.Average > 4.5 {
		t.Fatalf("report: anonymous %v, %d people, %d answers, average %.2f", r.Anonymous, r.Respondents, r.Responses, r.Average)
	}
	if len(r.Messages) < 100 || r.Kudos.Count < 50 || r.Participation == nil {
		t.Fatalf("report: %d messages, %d kudos, participation %v", len(r.Messages), r.Kudos.Count, r.Participation)
	}
	for _, m := range r.Messages {
		if m.MemberDiscordID != "" || m.Username != "" {
			t.Fatalf("the demo is anonymous, but a message names %q", m.Username)
		}
	}
	for m := domain.MoodAwful; m <= domain.MoodGreat; m++ {
		if r.Distribution[m] == 0 {
			t.Errorf("no answers with mood %d", m)
		}
	}
	// Only the demo server exists.
	if _, err := svc.Report(ctx, "100000000000000002", day("2026-10-01"), day("2026-10-31")); err != domain.ErrNotFound {
		t.Fatalf("other server: %v", err)
	}
}

// A day's answers don't change as time goes by, so the demo doesn't jump around from one visit
// to the next.
func TestDemoHistoryIsStable(t *testing.T) {
	ctx := context.Background()
	a := demo.NewStore("es", at("2026-10-28T15:00:00Z")).NewService()
	b := demo.NewStore("es", at("2026-11-04T15:00:00Z")).NewService()
	ra, _ := a.Report(ctx, demo.GuildID, day("2026-10-01"), day("2026-10-27"))
	rb, _ := b.Report(ctx, demo.GuildID, day("2026-10-01"), day("2026-10-27"))
	if !reflect.DeepEqual(ra.Days, rb.Days) || !reflect.DeepEqual(ra.Messages, rb.Messages) {
		t.Fatal("the same days gave different answers a week later")
	}
	// Today's answers only show once the survey went out at 13:00 in Madrid.
	before, _ := demo.NewStore("es", at("2026-10-28T10:00:00Z")).NewService().Report(ctx, demo.GuildID, day("2026-10-28"), day("2026-10-28"))
	after, _ := demo.NewStore("es", at("2026-10-28T15:00:00Z")).NewService().Report(ctx, demo.GuildID, day("2026-10-28"), day("2026-10-28"))
	if before.Responses != 0 || after.Responses == 0 {
		t.Fatalf("today: %d answers before the survey, %d after", before.Responses, after.Responses)
	}
}

func TestDemoViewer(t *testing.T) {
	ctx := context.Background()
	svc := demo.NewStore("es", at("2026-10-31T10:00:00Z")).NewService()
	v := demo.Viewer()
	gs, err := svc.VisibleGuilds(ctx, v)
	if err != nil || len(gs) != 1 || gs[0].Level != domain.AccessOwner || gs[0].Guild.Name != "Equipo Demo" {
		t.Fatalf("visible: %v %v", gs, err)
	}
	h, err := svc.GuildHealth(ctx, gs[0].Guild)
	if err != nil || h.Audience != 10 || h.OptedOut != 1 || h.LastSurvey == nil {
		t.Fatalf("health: %+v %v", h, err)
	}
	evs, err := svc.Evolution(ctx, v, day("2026-10-01"), day("2026-10-31"))
	if err != nil || len(evs) != 1 || evs[0].Responses == 0 {
		t.Fatalf("evolution: %v %v", evs, err)
	}
	// Saturday: this week's round is being guessed, and Laura left her guesses to the visitor.
	b, err := svc.GameBoard(ctx, v, demo.GuildID)
	if err != nil || b.Current == nil || b.Current.Phase != domain.GameGuessing || len(b.Current.Answers) < 3 || len(b.Current.MyGuesses) != 0 {
		t.Fatalf("game: %+v %v", b.Current, err)
	}
	if b.Previous == nil || b.Previous.Phase != domain.GameFinished || len(b.Previous.Ranking) == 0 || !b.Proposal.Open || b.Proposal.Count == 0 {
		t.Fatalf("game: previous %+v, proposal %+v", b.Previous, b.Proposal)
	}
	a, err := svc.GuildAccessSettings(ctx, v, demo.GuildID)
	if err != nil || len(a.Grants) == 0 || len(a.Roles) == 0 || len(a.Members) != 12 {
		t.Fatalf("access: %+v %v", a, err)
	}
}

// Nothing can be written: every write the dashboard could do fails.
func TestDemoIsReadOnly(t *testing.T) {
	ctx := context.Background()
	svc := demo.NewStore("es", at("2026-10-27T10:00:00Z")).NewService() // a Tuesday: answering is open
	v := demo.Viewer()
	anon := false
	if _, err := svc.UpdateGuildSettings(ctx, demo.GuildID, usecase.SettingsPatch{Anonymous: &anon}); err == nil {
		t.Fatal("settings changed")
	}
	if err := svc.DeleteGuild(ctx, v, demo.GuildID, "Equipo Demo"); err == nil {
		t.Fatal("server deleted")
	}
	if _, err := svc.SetGuildAccess(ctx, v, demo.GuildID, v.DiscordID, nil); err == nil {
		t.Fatal("access changed")
	}
	g, _ := svc.GetGuild(ctx, demo.GuildID)
	if !g.Settings.Anonymous {
		t.Fatal("the demo stopped being anonymous")
	}
}
