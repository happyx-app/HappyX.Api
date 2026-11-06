package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func recipients(t *testing.T, svc *usecase.Service, guild string, purpose usecase.MembersFor) map[string]bool {
	t.Helper()
	ms, err := svc.ListMembers(context.Background(), guild, purpose)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range ms {
		out[m.DiscordID] = true
	}
	return out
}

// TestVacations checks who gets the survey on a vacation, that admins only touch the ones set in
// their server, and that the bot is told to DM the person once.
func TestVacations(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) // a Friday
	svc := testService(t, now)
	mustRegister(t, svc, guildA)
	mustRegister(t, svc, guildB)
	for _, g := range []string{guildA, guildB} {
		mustJoin(t, svc, g, user1)
		mustJoin(t, svc, g, user2)
	}
	me := domain.Viewer{DiscordID: user1}
	owner := domain.Viewer{DiscordID: admin}

	// Their own vacation, from the web, covers every server.
	if _, err := svc.SetMyVacation(ctx, me, day("2026-11-06"), day("2026-11-10")); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{guildA, guildB} {
		for _, p := range []usecase.MembersFor{usecase.MembersSurvey, usecase.MembersGame} {
			if r := recipients(t, svc, g, p); r[user1] || !r[user2] {
				t.Fatalf("%s %s on vacation: %v", g, p, r)
			}
		}
		if r := recipients(t, svc, g, usecase.MembersAll); !r[user1] {
			t.Fatalf("vacation took them off the member list: %v", r)
		}
	}
	h, err := svc.GuildHealth(ctx, mustGuild(t, svc, guildA))
	if err != nil || h.Audience != 1 || h.OnVacation != 1 {
		t.Fatalf("health: %+v %v", h, err)
	}
	// Replacing it moves the dates; there is one own vacation.
	if _, err := svc.SetMyVacation(ctx, me, day("2026-11-09"), day("2026-11-10")); err != nil {
		t.Fatal(err)
	}
	if r := recipients(t, svc, guildA, usecase.MembersSurvey); !r[user1] {
		t.Fatal("old dates still apply")
	}
	if vs, _ := svc.MyVacations(ctx, me); len(vs) != 1 || !vs[0].From.Equal(day("2026-11-09")) {
		t.Fatalf("own vacations: %+v", vs)
	}
	if _, err := svc.SetMyVacation(ctx, me, day("2026-11-01"), day("2026-11-04")); err == nil {
		t.Fatal("a vacation that already ended was accepted")
	}

	// An admin sets one for user2 in guild A only, and the bot gets a notice for it.
	if _, err := svc.SetGuildVacation(ctx, owner, guildA, user2, day("2026-11-06"), day("2026-11-06")); err != nil {
		t.Fatal(err)
	}
	if r := recipients(t, svc, guildA, usecase.MembersSurvey); r[user2] {
		t.Fatal("admin-set vacation ignored")
	}
	if r := recipients(t, svc, guildB, usecase.MembersSurvey); !r[user2] {
		t.Fatal("admin-set vacation leaked to another server")
	}
	if _, err := svc.SetGuildVacation(ctx, me, guildA, user2, day("2026-11-06"), day("2026-11-06")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a member without access set a vacation: %v", err)
	}
	gv, err := svc.ListGuildVacations(ctx, owner, guildA)
	if err != nil || len(gv.Vacations) != 2 || len(gv.Members) != 2 {
		t.Fatalf("guild vacations: %+v %v", gv, err)
	}
	if v := gv.Vacations[0]; v.UserDiscordID != user2 || v.GuildDiscordID != guildA || v.Username == "" {
		t.Fatalf("first vacation should be user2's, set here: %+v", v)
	}
	notices, err := svc.VacationNotices(ctx)
	if err != nil || len(notices) != 1 || notices[0].UserDiscordID != user2 || notices[0].SetByDiscordID != admin || notices[0].GuildDiscordID != guildA {
		t.Fatalf("notices: %+v %v", notices, err)
	}
	if err := svc.DoneVacationNotice(ctx, notices[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DoneVacationNotice(ctx, notices[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("notice done twice: %v", err)
	}
	// Admins can't remove someone's own vacation, only the one set in their server.
	if err := svc.DeleteGuildVacation(ctx, owner, guildA, user1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("admin removed an own vacation: %v", err)
	}
	// The person can remove the one an admin set for them.
	if err := svc.DeleteMyVacation(ctx, domain.Viewer{DiscordID: user2}, guildA); err != nil {
		t.Fatal(err)
	}
	if r := recipients(t, svc, guildA, usecase.MembersSurvey); !r[user2] {
		t.Fatal("removed vacation still applies")
	}

	// /happyx vacation and /happyx resume from the bot.
	if _, err := svc.SetMemberOwnVacation(ctx, guildB, user2, day("2026-11-06"), day("2026-11-07")); err != nil {
		t.Fatal(err)
	}
	if vs, err := svc.MemberVacations(ctx, guildA, user2); err != nil || len(vs) != 1 {
		t.Fatalf("own vacation from the bot should apply everywhere: %+v %v", vs, err)
	}
	if err := svc.EndMemberVacations(ctx, guildA, user2); err != nil {
		t.Fatal(err)
	}
	if vs, _ := svc.MemberVacations(ctx, guildB, user2); len(vs) != 0 {
		t.Fatalf("resume left vacations: %+v", vs)
	}

	// Erasing someone's data takes their vacations.
	if err := svc.EraseMyData(ctx, domain.Viewer{DiscordID: user1, Username: "user1"}, "user1"); err != nil {
		t.Fatal(err)
	}
	if vs, _ := svc.MyVacations(ctx, me); len(vs) != 0 {
		t.Fatalf("erase kept vacations: %+v", vs)
	}
}

// A vacation keeps the streak going, in the database and on the person's own page alike.
func TestVacationKeepsStreak(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) // a Friday
	svc := testService(t, now)
	mustRegister(t, svc, guildA)
	mustJoin(t, svc, guildA, user1)
	for _, d := range []string{"2026-10-26", "2026-10-27", "2026-11-05", "2026-11-06"} {
		if _, err := svc.RecordMood(ctx, usecase.RecordMoodInput{GuildID: guildA, UserID: user1, Mood: domain.MoodGood, Day: day(d)}); err != nil {
			t.Fatal(err)
		}
	}
	streak := func() (db, page int) {
		got, err := svc.MemberStreaks(ctx, guildA)
		if err != nil {
			t.Fatal(err)
		}
		evs, err := svc.Evolution(ctx, domain.Viewer{DiscordID: user1}, day("2026-10-01"), day("2026-11-06"))
		if err != nil || len(evs) != 1 {
			t.Fatalf("evolution: %+v %v", evs, err)
		}
		return got[user1], evs[0].Streak
	}
	if db, page := streak(); db != 2 || page != 2 {
		t.Fatalf("without vacation: %d %d", db, page)
	}
	// Set a vacation that already ended straight in the table: the API refuses to set those.
	if _, err := NewVacationRepo(pgPool(t)).Set(ctx, domain.Vacation{UserDiscordID: user1, From: day("2026-10-28"), To: day("2026-11-04"), SetByDiscordID: user1}, false); err != nil {
		t.Fatal(err)
	}
	if db, page := streak(); db != 4 || page != 4 {
		t.Fatalf("with vacation: %d %d", db, page)
	}
	// It is kept while it can matter to a streak.
	if n, err := svc.DeleteOldVacations(ctx); err != nil || n != 0 {
		t.Fatalf("deleted a recent vacation: %d %v", n, err)
	}
}

func mustGuild(t *testing.T, svc *usecase.Service, id string) domain.Guild {
	t.Helper()
	g, err := svc.GetGuild(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// pgPool opens a pool on the test database without resetting it.
func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := Connect(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
