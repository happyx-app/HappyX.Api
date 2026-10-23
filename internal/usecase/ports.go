// Package usecase implements HappyX's application logic. It depends only on the domain
// and on the repository interfaces declared here; storage lives in adapters.
package usecase

import (
	"context"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// Every repository method that reads or writes tenant data takes the guild's Discord ID,
// so implementations can (and must) scope each query to a single guild.

type GuildRepository interface {
	// Upsert creates the guild or, if it exists, updates its name and admin and reactivates it.
	// Existing settings are kept on update.
	Upsert(ctx context.Context, g domain.Guild) (domain.Guild, error)
	Get(ctx context.Context, guildID string) (domain.Guild, error)
	ListActive(ctx context.Context) ([]domain.Guild, error)
	// UpdateConfig sets the guild's admin and settings in one write.
	UpdateConfig(ctx context.Context, guildID, adminID string, s domain.GuildSettings) (domain.Guild, error)
	Deactivate(ctx context.Context, guildID string) error
}

type MemberRepository interface {
	Upsert(ctx context.Context, m domain.Member) (domain.Member, error)
	Get(ctx context.Context, guildID, userID string) (domain.Member, error)
	ListActive(ctx context.Context, guildID string) ([]domain.Member, error)
	Deactivate(ctx context.Context, guildID, userID string) error
}

type MoodRepository interface {
	// Upsert stores the member's mood for e.Day, replacing an earlier answer for that day.
	Upsert(ctx context.Context, e domain.MoodEntry) (domain.MoodEntry, error)
	ListBetween(ctx context.Context, guildID string, from, to time.Time) ([]domain.MoodEntry, error)
}

// Clock lets tests control "today".
type Clock func() time.Time
