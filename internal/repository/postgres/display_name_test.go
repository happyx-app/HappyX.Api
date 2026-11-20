package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// The name the server shows comes back wherever a member is named, falls back to the username
// until the bot reports it, survives callers that only know the username, and goes with an
// erase.
func TestMemberDisplayName(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	svc := testService(t, now)
	mustRegister(t, svc, guildA)

	// Before the bot reports it, the username stands in.
	m, err := svc.UpsertMember(ctx, guildA, user1, "ana.garcia_corp", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.DisplayName != "ana.garcia_corp" {
		t.Fatalf("display name falls back to the username: %+v", m)
	}
	if err := svc.SyncMembers(ctx, guildA, []domain.Member{
		{DiscordID: user1, Username: "ana.garcia_corp", DisplayName: "Ana"},
		{DiscordID: user2, Username: "bea_corp", DisplayName: "Bea"},
	}); err != nil {
		t.Fatal(err)
	}
	// Joining again without a display name (an older bot, or a caller that only knows the
	// username) keeps the one reported.
	if m, err = svc.UpsertMember(ctx, guildA, user1, "ana.garcia_corp", "", nil, nil); err != nil || m.DisplayName != "Ana" {
		t.Fatalf("an empty display name keeps the stored one: %+v, %v", m, err)
	}
	if m, err = svc.UpsertMember(ctx, guildA, user2, "bea_corp", "Beatriz", nil, nil); err != nil || m.DisplayName != "Beatriz" {
		t.Fatalf("a new display name replaces it: %+v, %v", m, err)
	}
	ms, err := svc.ListMembers(ctx, guildA, usecase.MembersAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].DisplayName != "Ana" || ms[1].DisplayName != "Beatriz" {
		t.Fatalf("members with their display names: %+v", ms)
	}

	// Two members going by the same name are told apart by their usernames, wherever they show.
	if err := svc.SyncMembers(ctx, guildA, []domain.Member{{DiscordID: user3, Username: "david.garcia", DisplayName: "David"}}); err != nil {
		t.Fatal(err)
	}
	m, err = svc.UpsertMember(ctx, guildA, "200000000000000004", "david.moreno", "david", nil, nil)
	if err != nil || m.DisplayName != "david (david.moreno)" {
		t.Fatalf("a namesake, ignoring case: %+v, %v", m, err)
	}
	if m, _ = svc.UpsertMember(ctx, guildA, user3, "david.garcia", "", nil, nil); m.DisplayName != "David (david.garcia)" {
		t.Fatalf("both namesakes: %+v", m)
	}
	if m, _ = svc.UpsertMember(ctx, guildA, "200000000000000004", "david.moreno", "Dave", nil, nil); m.DisplayName != "Dave" {
		t.Fatalf("a new nickname ends the clash: %+v", m)
	}

	k, err := svc.GiveKudos(ctx, usecase.GiveKudosInput{GuildID: guildA, FromUserID: user1, ToUserID: user2, Message: "gracias"})
	if err != nil {
		t.Fatal(err)
	}
	if k.GiverDisplayName != "Ana" || k.ReceiverDisplayName != "Beatriz" || k.GiverUsername != "ana.garcia_corp" {
		t.Fatalf("kudos name both members as the server shows them: %+v", k)
	}
	day := domain.TruncateDay(now)
	r, err := svc.Report(ctx, guildA, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Kudos.TopReceivers) != 1 || r.Kudos.TopReceivers[0].DisplayName != "Beatriz" {
		t.Fatalf("top receivers by display name: %+v", r.Kudos.TopReceivers)
	}

	if err := svc.EraseMyData(ctx, domain.Viewer{DiscordID: user1, Username: "Ana"}, "Ana"); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var username, display string
	if err := pool.QueryRow(ctx, `SELECT username, display_name FROM members WHERE discord_id = $1`, user1).Scan(&username, &display); err != nil {
		t.Fatal(err)
	}
	if username != "" || display != "" {
		t.Fatalf("erasing keeps a name: %q, %q", username, display)
	}
}
