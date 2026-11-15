package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// ChannelPostRepo stores the messages HappyX keeps in a channel. Every query is scoped to one
// guild by its Discord ID.
type ChannelPostRepo struct{ db *pgxpool.Pool }

func NewChannelPostRepo(db *pgxpool.Pool) *ChannelPostRepo { return &ChannelPostRepo{db: db} }

func (r *ChannelPostRepo) Get(ctx context.Context, guildID, kind string) (domain.ChannelPost, error) {
	p := domain.ChannelPost{GuildDiscordID: guildID, Kind: kind}
	err := r.db.QueryRow(ctx, `
		SELECT p.channel_id, p.message_id, p.created_at
		FROM channel_posts p JOIN guilds g ON g.id = p.guild_id
		WHERE g.discord_id = $1 AND p.kind = $2`, guildID, kind).Scan(&p.ChannelID, &p.MessageID, &p.CreatedAt)
	return p, notFound(err)
}

func (r *ChannelPostRepo) Put(ctx context.Context, p domain.ChannelPost) (domain.ChannelPost, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO channel_posts (guild_id, kind, channel_id, message_id)
		SELECT id, $2, $3, $4 FROM guilds WHERE discord_id = $1
		ON CONFLICT (guild_id, kind) DO UPDATE
		SET channel_id = EXCLUDED.channel_id, message_id = EXCLUDED.message_id, created_at = now()
		RETURNING created_at`, p.GuildDiscordID, p.Kind, p.ChannelID, p.MessageID).Scan(&p.CreatedAt)
	return p, notFound(err)
}

func (r *ChannelPostRepo) Delete(ctx context.Context, guildID, kind string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM channel_posts p USING guilds g
		WHERE g.id = p.guild_id AND g.discord_id = $1 AND p.kind = $2`, guildID, kind)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
