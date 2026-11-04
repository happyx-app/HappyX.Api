package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// TestNewGuildStartsWithTheSurveyOff checks a server the bot just joined doesn't get the survey
// until it is started, and that starting it is saved and survives the bot reconnecting.
func TestNewGuildStartsWithTheSurveyOff(t *testing.T) {
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
	join := domain.Guild{DiscordID: guildA, Name: "G", AdminDiscordID: admin, Settings: domain.DefaultGuildSettings()}
	g, err := guilds.Upsert(ctx, join)
	if err != nil {
		t.Fatal(err)
	}
	if g.Settings.SurveyEnabled {
		t.Fatal("a new guild starts with the survey on")
	}
	st := g.Settings
	st.SurveyEnabled = true
	if g, err = guilds.UpdateConfig(ctx, guildA, admin, st); err != nil || !g.Settings.SurveyEnabled {
		t.Fatalf("start survey: %v %v", g.Settings.SurveyEnabled, err)
	}
	if g, err = guilds.Upsert(ctx, join); err != nil || !g.Settings.SurveyEnabled {
		t.Fatalf("a reconnect turned the survey off: %v %v", g.Settings.SurveyEnabled, err)
	}
}
