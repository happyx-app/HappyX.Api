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
	// Resuming ends it: see TestResumeEndsVacations.
	if _, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil {
		t.Fatal(err)
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
	clock := &testClock{now: now}
	svc := testServiceClock(t, clock.Now)
	mustRegister(t, svc, guildA)
	mustJoin(t, svc, guildA, user1)
	for _, d := range []string{"2026-10-26", "2026-10-27", "2026-11-05", "2026-11-06"} {
		answerOn(t, svc, clock, guildA, user1, day(d), domain.MoodGood)
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
	if _, err := NewVacationRepo(pgPool(t)).Set(ctx, domain.Vacation{UserDiscordID: user1, From: day("2026-10-28"), To: day("2026-11-04"), SetByDiscordID: user1}, false, day("2026-11-05")); err != nil {
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

// Resuming ends the vacations that cover today, whoever set them, and leaves later ones alone.
// Until the bot records today's survey as sent, today is a working day again and the survey reaches
// the member; once it went out without them, today stays vacation so it doesn't count as a day not
// answered. What counts is the recorded send, not the survey hour.
func TestResumeEndsVacations(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)} // Friday, 9:00 in Madrid; the survey goes at 13:00
	svc := testServiceClock(t, clock.Now)
	mustRegister(t, svc, guildA)
	mustStartSurvey(t, svc, guildA)
	mustJoin(t, svc, guildA, user2)
	owner := domain.Viewer{DiscordID: admin}
	set := func() {
		t.Helper()
		if _, err := svc.SetMemberOwnVacation(ctx, guildA, user2, day("2026-11-04"), day("2026-11-10")); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetGuildVacation(ctx, owner, guildA, user2, day("2026-11-20"), day("2026-11-25")); err != nil {
			t.Fatal(err)
		}
	}
	sent := func(d time.Time) {
		t.Helper()
		if ok, err := svc.ClaimDispatch(ctx, guildA, domain.DispatchSurvey, d); err != nil || !ok {
			t.Fatalf("claim the survey: %v %v", ok, err)
		}
	}
	mine := func() []domain.Vacation {
		t.Helper()
		vs, err := svc.MyVacations(ctx, domain.Viewer{DiscordID: user2})
		if err != nil {
			t.Fatal(err)
		}
		return vs
	}

	// Before the survey: the vacation ends yesterday and today's survey reaches them.
	set()
	ended, err := svc.EndMemberVacations(ctx, guildA, user2)
	if err != nil || len(ended) != 1 || ended[0].GuildDiscordID != "" || !ended[0].To.Equal(day("2026-11-10")) {
		t.Fatalf("resume during a vacation: %+v %v", ended, err)
	}
	if r := recipients(t, svc, guildA, usecase.MembersSurvey); !r[user2] {
		t.Fatal("back from vacation but no survey today")
	}
	if vs := mine(); len(vs) != 2 || !vs[0].To.Equal(day("2026-11-05")) || !vs[1].From.Equal(day("2026-11-20")) {
		t.Fatalf("after resume: %+v", vs)
	}
	if ended, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil || len(ended) != 0 {
		t.Fatalf("nothing to end: %+v %v", ended, err)
	}

	// Past the survey hour but not sent yet (late, or given back after reaching nobody): the survey
	// can still reach them, so the vacation ends yesterday.
	clock.now = time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC) // 18:00 in Madrid
	set()
	if ended, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil || len(ended) != 1 {
		t.Fatalf("resume before the late survey: %+v %v", ended, err)
	}
	if vs := mine(); len(vs) != 2 || !vs[0].To.Equal(day("2026-11-05")) {
		t.Fatalf("after resuming before the late survey: %+v", vs)
	}

	// After the survey went out: today stays vacation, the rest of it ends.
	sent(day("2026-11-06"))
	set()
	if ended, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil || len(ended) != 1 {
		t.Fatalf("resume after the survey: %+v %v", ended, err)
	}
	if vs := mine(); len(vs) != 2 || !vs[0].To.Equal(day("2026-11-06")) || !vs[1].From.Equal(day("2026-11-20")) {
		t.Fatalf("after resuming in the evening, today stays vacation: %+v", vs)
	}

	// One starting today: removed before the survey, kept for today after it.
	clock.now = time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC) // Monday morning
	if _, err := svc.SetMemberOwnVacation(ctx, guildA, user2, day("2026-11-09"), day("2026-11-12")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil {
		t.Fatal(err)
	}
	if vs := mine(); len(vs) != 1 || !vs[0].From.Equal(day("2026-11-20")) {
		t.Fatalf("one starting today, resumed in the morning, is gone: %+v", vs)
	}
	sent(day("2026-11-09"))
	if _, err := svc.SetMemberOwnVacation(ctx, guildA, user2, day("2026-11-09"), day("2026-11-12")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndMemberVacations(ctx, guildA, user2); err != nil {
		t.Fatal(err)
	}
	if vs := mine(); len(vs) != 2 || !vs[0].From.Equal(day("2026-11-09")) || !vs[0].To.Equal(day("2026-11-09")) {
		t.Fatalf("one starting today, resumed in the evening, keeps today: %+v", vs)
	}
}

// «My evolution» gets the days the survey went out since the person joined, and their days off,
// replaced ones included, while the vacations page only shows the current ones.
func TestEvolutionSurveyDaysAndVacationHistory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) // a Friday
	clock := &testClock{now: now}
	svc := testServiceClock(t, clock.Now)
	mustRegister(t, svc, guildA)
	mustJoin(t, svc, guildA, user1)
	pool := pgPool(t)
	// They joined on 1 October: the survey of 30 September wasn't theirs.
	if _, err := pool.Exec(ctx, `UPDATE members SET created_at = '2026-10-01 09:00+00' WHERE discord_id = $1`, user1); err != nil {
		t.Fatal(err)
	}
	dispatches := NewDispatchRepo(pool)
	for d, n := range map[string]int{"2026-09-30": 3, "2026-11-02": 3, "2026-11-03": 3, "2026-11-04": 0, "2026-11-05": 3} {
		if _, err := dispatches.Claim(ctx, guildA, domain.DispatchSurvey, day(d), 3); err != nil {
			t.Fatal(err)
		}
		if err := dispatches.Finish(ctx, guildA, domain.DispatchSurvey, day(d), n, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	// A survey claimed but never finished didn't reach anyone yet.
	if _, err := dispatches.Claim(ctx, guildA, domain.DispatchSurvey, day("2026-11-06"), 3); err != nil {
		t.Fatal(err)
	}
	answerOn(t, svc, clock, guildA, user1, day("2026-11-05"), domain.MoodGood)

	// A vacation that started before the API's today, replaced by a later one: its days gone stay.
	vacations := NewVacationRepo(pool)
	if _, err := vacations.Set(ctx, domain.Vacation{UserDiscordID: user1, From: day("2026-11-02"), To: day("2026-11-08"), SetByDiscordID: user1}, false, day("2026-11-01")); err != nil {
		t.Fatal(err)
	}
	me := domain.Viewer{DiscordID: user1}
	if _, err := svc.SetMyVacation(ctx, me, day("2026-11-12"), day("2026-11-13")); err != nil {
		t.Fatal(err)
	}
	if vs, err := svc.MyVacations(ctx, me); err != nil || len(vs) != 1 || !vs[0].From.Equal(day("2026-11-12")) {
		t.Fatalf("current vacations: %+v %v", vs, err)
	}

	evs, err := svc.Evolution(ctx, me, day("2026-09-28"), day("2026-11-06"))
	if err != nil || len(evs) != 1 {
		t.Fatalf("evolution: %+v %v", evs, err)
	}
	var got []string
	for _, d := range evs[0].SurveyDays {
		got = append(got, d.Format("2006-01-02"))
	}
	if want := []string{"2026-11-02", "2026-11-03", "2026-11-05"}; !equalStrings(got, want) {
		t.Fatalf("survey days: %v, want %v", got, want)
	}
	if vs := evs[0].Vacations; len(vs) != 1 || !vs[0].From.Equal(day("2026-11-02")) || !vs[0].To.Equal(day("2026-11-04")) {
		t.Fatalf("vacations: %+v", vs)
	}

	// Removing the current one keeps the history too, and it can't be removed twice.
	if err := svc.DeleteMyVacation(ctx, me, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMyVacation(ctx, me, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if vs, _ := svc.MyVacations(ctx, me); len(vs) != 0 {
		t.Fatalf("deleted vacation still listed: %+v", vs)
	}
	if evs, _ := svc.Evolution(ctx, me, day("2026-09-28"), day("2026-11-06")); len(evs) != 1 || len(evs[0].Vacations) != 1 {
		t.Fatalf("history lost: %+v", evs)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
