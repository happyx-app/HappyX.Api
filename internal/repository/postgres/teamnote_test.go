package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// TestTeamNotes publishes, edits and deletes what an admin tells the team, checks which day's
// thanks card points to it, that another guild can't reach it, and that erasing its author's
// data takes it.
func TestTeamNotes(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)} // a Monday, 16:00 in Madrid
	svc := testServiceClock(t, clock.Now)
	mustRegister(t, svc, guildA)
	mustRegister(t, svc, guildB)
	mustJoin(t, svc, guildA, admin)

	note := domain.TeamNote{
		GuildDiscordID: guildA, AuthorDiscordID: admin, From: day("2026-11-09"), To: day("2026-11-15"),
		Body:      "  Muchas reuniones seguidas. Desde el lunes, martes por la tarde sin reuniones.  ",
		ChannelID: "400000000000000001", MessageID: "500000000000000001",
	}
	// Only a member of the guild can be its author.
	other := note
	other.AuthorDiscordID = user3
	if _, err := svc.PublishTeamNote(ctx, other); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("author outside the guild: %v", err)
	}
	n, err := svc.PublishTeamNote(ctx, note)
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "Muchas reuniones seguidas. Desde el lunes, martes por la tarde sin reuniones." || !n.PostedOn.Equal(day("2026-11-16")) || n.AuthorUsername == "" {
		t.Fatalf("published: %+v", n)
	}

	// The thanks card points to it on the next survey day only.
	for d, want := range map[string]bool{"2026-11-16": false, "2026-11-17": true, "2026-11-18": false} {
		_, err := svc.CurrentTeamNote(ctx, guildA, day(d))
		if got := err == nil; got != want {
			t.Errorf("current on %s: %v, want found=%v", d, err, want)
		}
	}
	// Published on a Friday, it is news on Monday.
	clock.now = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	friday, err := svc.PublishTeamNote(ctx, note)
	if err != nil {
		t.Fatal(err)
	}
	if cur, err := svc.CurrentTeamNote(ctx, guildA, day("2026-11-23")); err != nil || cur.ID != friday.ID {
		t.Fatalf("Monday after a Friday note: %+v %v", cur, err)
	}

	// Another guild can neither see nor change it.
	if _, err := svc.TeamNote(ctx, guildB, n.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("read from another guild: %v", err)
	}
	if _, err := svc.EditTeamNote(ctx, guildB, n.ID, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("edit from another guild: %v", err)
	}
	if err := svc.DeleteTeamNote(ctx, guildB, n.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete from another guild: %v", err)
	}

	if _, err := svc.EditTeamNote(ctx, guildA, n.ID, "   "); err == nil {
		t.Fatal("an empty note was accepted")
	}
	if e, err := svc.EditTeamNote(ctx, guildA, n.ID, "Martes sin reuniones."); err != nil || e.Body != "Martes sin reuniones." {
		t.Fatalf("edit: %+v %v", e, err)
	}
	if err := svc.DeleteTeamNote(ctx, guildA, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TeamNote(ctx, guildA, n.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted note still there: %v", err)
	}

	// It is in its author's data, and goes when they erase it.
	me := domain.Viewer{DiscordID: admin}
	data, err := svc.MyData(ctx, me)
	if err != nil || len(data.Guilds) != 1 || len(data.Guilds[0].TeamNotes) != 1 {
		t.Fatalf("my data: %+v %v", data, err)
	}
	me.Username = data.Guilds[0].Member.Username
	if err := svc.EraseMyData(ctx, me, me.Username); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TeamNote(ctx, guildA, friday.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("erased author's note still there: %v", err)
	}
}

// TestReportAudience: the report says how many people get the survey now, to compare with how
// many answered. Someone on vacation is not counted.
func TestReportAudience(t *testing.T) {
	ctx := context.Background()
	svc := testService(t, time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC))
	mustRegister(t, svc, guildA)
	for _, u := range []string{user1, user2, user3} {
		mustJoin(t, svc, guildA, u)
	}
	if _, err := svc.SetMyVacation(ctx, domain.Viewer{DiscordID: user3}, day("2026-11-16"), day("2026-11-20")); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Report(ctx, guildA, day("2026-11-09"), day("2026-11-15"))
	if err != nil || r.Audience != 2 {
		t.Fatalf("audience: %d %v", r.Audience, err)
	}
}
