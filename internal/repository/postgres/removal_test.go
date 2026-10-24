package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

// TestLeftGuildIsKeptThenDeleted checks a guild the bot left disappears from the dashboard,
// comes back whole if the bot is added again, and is deleted after the retention period.
func TestLeftGuildIsKeptThenDeleted(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	svc := testServiceClock(t, func() time.Time { return now })
	mustRegister(t, svc, guildA)
	mustRegister(t, svc, guildB)
	for _, g := range []string{guildA, guildB} {
		mustJoin(t, svc, g, user1)
		if _, err := svc.RecordMood(ctx, usecase.RecordMoodInput{GuildID: g, UserID: user1, Mood: domain.MoodGood}); err != nil {
			t.Fatal(err)
		}
	}
	viewer := domain.Viewer{DiscordID: admin}

	if err := svc.RemoveGuild(ctx, guildA); err != nil {
		t.Fatal(err)
	}
	if gs, _ := svc.VisibleGuilds(ctx, viewer); len(gs) != 1 || gs[0].DiscordID != guildB {
		t.Fatalf("a guild the bot left must be hidden from the dashboard: %v", gs)
	}
	if _, err := svc.ViewerGuild(ctx, viewer, guildA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reading a guild the bot left: want ErrNotFound, got %v", err)
	}

	// Added back within the retention period: everything is still there.
	now = now.Add(20 * 24 * time.Hour)
	mustRegister(t, svc, guildA)
	if r, err := svc.Report(ctx, guildA, domain.TruncateDay(now.Add(-20*24*time.Hour)), domain.TruncateDay(now)); err != nil || r.Responses != 1 {
		t.Fatalf("data lost after re-adding the bot: %+v %v", r, err)
	}

	// Removed again: the retention counts from this removal, not the first one.
	if err := svc.RemoveGuild(ctx, guildA); err != nil {
		t.Fatal(err)
	}
	now = now.Add(usecase.LeftGuildRetention - time.Hour)
	if ids, err := svc.DeleteLeftGuilds(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("deleted before the retention period: %v %v", ids, err)
	}
	now = now.Add(2 * time.Hour)
	if ids, err := svc.DeleteLeftGuilds(ctx); err != nil || len(ids) != 1 || ids[0] != guildA {
		t.Fatalf("delete after the retention period: %v %v", ids, err)
	}
	if _, err := svc.GetGuild(ctx, guildA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("guild A should be gone: %v", err)
	}
	if r, err := svc.Report(ctx, guildB, domain.TruncateDay(now.Add(-60*24*time.Hour)), domain.TruncateDay(now)); err != nil || r.Responses != 1 {
		t.Fatalf("guild B must be untouched: %+v %v", r, err)
	}
	// The bot isn't in the server any more, so it has nothing to leave.
	if ids, err := svc.GuildRemovals(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("removal queue: %v %v", ids, err)
	}
}

// TestDeleteGuild checks deleting a guild from the dashboard removes all its data, keeps it from
// registering again until the bot has left, and leaves other guilds alone.
func TestDeleteGuild(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	svc := testService(t, now)
	if _, err := svc.RegisterGuild(ctx, usecase.RegisterGuildInput{DiscordID: guildA, Name: "Equipo A", AdminDiscordID: admin}); err != nil {
		t.Fatal(err)
	}
	mustRegister(t, svc, guildB)
	for _, g := range []string{guildA, guildB} {
		mustJoin(t, svc, g, user1)
		mustJoin(t, svc, g, user2)
		if _, err := svc.RecordMood(ctx, usecase.RecordMoodInput{GuildID: g, UserID: user1, Mood: domain.MoodGood}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.GiveKudos(ctx, usecase.GiveKudosInput{GuildID: g, FromUserID: user1, ToUserID: user2, Message: "gracias"}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteGuild(ctx, domain.Viewer{DiscordID: user2}, guildA, "Equipo A"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a member who can't see the dashboard deleted the guild: %v", err)
	}
	owner := domain.Viewer{DiscordID: admin}
	if err := svc.DeleteGuild(ctx, owner, guildA, "Equipo B"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("wrong confirmation: want ErrInvalid, got %v", err)
	}
	if err := svc.DeleteGuild(ctx, owner, guildA, " Equipo A "); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetGuild(ctx, guildA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("guild A should be gone: %v", err)
	}
	if ids, err := svc.GuildRemovals(ctx); err != nil || len(ids) != 1 || ids[0] != guildA {
		t.Fatalf("removal queue: %v %v", ids, err)
	}
	// The bot reconnecting before it has left mustn't bring the guild back.
	if _, err := svc.RegisterGuild(ctx, usecase.RegisterGuildInput{DiscordID: guildA, Name: "Equipo A", AdminDiscordID: admin}); !errors.Is(err, domain.ErrRemoving) {
		t.Fatalf("register while removing: want ErrRemoving, got %v", err)
	}

	day := domain.TruncateDay(now)
	if r, err := svc.Report(ctx, guildB, day, day); err != nil || r.Responses != 1 || r.Kudos.Count != 1 {
		t.Fatalf("guild B must be untouched: %+v %v", r, err)
	}

	// Once the bot has left, inviting it again starts from scratch.
	if err := svc.GuildRemoved(ctx, guildA); err != nil {
		t.Fatal(err)
	}
	if ids, _ := svc.GuildRemovals(ctx); len(ids) != 0 {
		t.Fatalf("removal queue after leaving: %v", ids)
	}
	mustRegister(t, svc, guildA)
	mustJoin(t, svc, guildA, user1)
	if r, err := svc.Report(ctx, guildA, day, day); err != nil || r.Responses != 0 || r.Kudos.Count != 0 {
		t.Fatalf("a re-added guild must start empty: %+v %v", r, err)
	}
}
