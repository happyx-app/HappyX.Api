package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Team events ---

// TeamEventRepo stores the events marked on a guild's chart, scoped to the guild like any tenant
// row: every query joins on the guild's Discord id.
type TeamEventRepo struct{ db *pgxpool.Pool }

func NewTeamEventRepo(db *pgxpool.Pool) *TeamEventRepo { return &TeamEventRepo{db: db} }

func scanTeamEvent(row pgx.Row) (domain.TeamEvent, error) {
	var e domain.TeamEvent
	err := row.Scan(&e.ID, &e.GuildDiscordID, &e.From, &e.To, &e.Title, &e.Description, &e.Color)
	e.From, e.To = domain.TruncateDay(e.From), domain.TruncateDay(e.To)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, domain.ErrNotFound
	}
	return e, err
}

func (r *TeamEventRepo) ForGuild(ctx context.Context, guildID string, from, to time.Time) ([]domain.TeamEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, g.discord_id, e.starts_on, e.ends_on, e.title, e.description, e.color
		FROM team_events e JOIN guilds g ON g.id = e.guild_id
		WHERE g.discord_id = $1 AND e.ends_on >= $2 AND e.starts_on <= $3
		ORDER BY e.starts_on, e.ends_on, e.id`, guildID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TeamEvent{}
	for rows.Next() {
		e, err := scanTeamEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *TeamEventRepo) Count(ctx context.Context, guildID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM team_events e JOIN guilds g ON g.id = e.guild_id
		WHERE g.discord_id = $1`, guildID).Scan(&n)
	return n, err
}

func (r *TeamEventRepo) Create(ctx context.Context, e domain.TeamEvent) (domain.TeamEvent, error) {
	return scanTeamEvent(r.db.QueryRow(ctx, `
		INSERT INTO team_events (guild_id, starts_on, ends_on, title, description, color)
		SELECT g.id, $2, $3, $4, $5, $6 FROM guilds g WHERE g.discord_id = $1
		RETURNING id, $1::text, starts_on, ends_on, title, description, color`,
		e.GuildDiscordID, e.From, e.To, e.Title, e.Description, e.Color))
}

func (r *TeamEventRepo) Update(ctx context.Context, e domain.TeamEvent) (domain.TeamEvent, error) {
	return scanTeamEvent(r.db.QueryRow(ctx, `
		UPDATE team_events e SET starts_on = $3, ends_on = $4, title = $5, description = $6, color = $7, updated_at = now()
		FROM guilds g
		WHERE g.id = e.guild_id AND g.discord_id = $1 AND e.id = $2
		RETURNING e.id, g.discord_id, e.starts_on, e.ends_on, e.title, e.description, e.color`,
		e.GuildDiscordID, e.ID, e.From, e.To, e.Title, e.Description, e.Color))
}

func (r *TeamEventRepo) Delete(ctx context.Context, guildID string, id int64) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM team_events e USING guilds g
		WHERE g.id = e.guild_id AND g.discord_id = $1 AND e.id = $2`, guildID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
