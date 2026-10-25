package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// TestGuildDeletionsAreCounted checks deleting a guild leaves a dated line for the admin
// dashboard's counts, with nothing that says which guild it was.
func TestGuildDeletionsAreCounted(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reset(t, pool)
	guilds := NewGuildRepo(pool)
	for _, id := range []string{guildA, guildB} {
		if _, err := guilds.Upsert(ctx, domain.Guild{DiscordID: id, Name: "G", AdminDiscordID: admin, Settings: domain.DefaultGuildSettings()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := guilds.Delete(ctx, guildA); err != nil {
		t.Fatal(err)
	}
	left := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := guilds.Deactivate(ctx, guildB, left); err != nil {
		t.Fatal(err)
	}
	if ids, err := guilds.DeleteLeftBefore(ctx, left.Add(time.Hour)); err != nil || len(ids) != 1 {
		t.Fatalf("delete left: %v %v", ids, err)
	}
	var dashboard, retention int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE reason = 'dashboard'), count(*) FILTER (WHERE reason = 'retention') FROM guild_deletions`).Scan(&dashboard, &retention); err != nil {
		t.Fatal(err)
	}
	if dashboard != 1 || retention != 1 {
		t.Fatalf("deletions: dashboard %d, retention %d; want 1 and 1", dashboard, retention)
	}
}
