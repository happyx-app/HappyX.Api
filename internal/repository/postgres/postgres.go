// Package postgres implements the usecase repositories on PostgreSQL.
//
// Tenant isolation: every query on members and mood_entries joins or filters on the
// guild's Discord ID, so a caller can only ever see rows of the guild it names.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/migrations"
)

// Connect opens a pool and applies pending migrations.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if err := Migrate(pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func Migrate(pool *pgxpool.Pool) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("migrations driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	// Close releases the connection the driver holds; without it pool.Close blocks forever.
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// --- Guilds ---

type GuildRepo struct{ db *pgxpool.Pool }

func NewGuildRepo(db *pgxpool.Pool) *GuildRepo { return &GuildRepo{db: db} }

const guildCols = `discord_id, name, admin_discord_id, anonymous, timezone, survey_hour,
	report_frequency, language, active, created_at, updated_at`

func scanGuild(row pgx.Row) (domain.Guild, error) {
	var g domain.Guild
	var freq string
	err := row.Scan(&g.DiscordID, &g.Name, &g.AdminDiscordID, &g.Settings.Anonymous,
		&g.Settings.Timezone, &g.Settings.SurveyHour, &freq, &g.Settings.Language, &g.Active, &g.CreatedAt, &g.UpdatedAt)
	g.Settings.ReportFrequency = domain.ReportFrequency(freq)
	return g, notFound(err)
}

func (r *GuildRepo) Upsert(ctx context.Context, g domain.Guild) (domain.Guild, error) {
	s := g.Settings
	return scanGuild(r.db.QueryRow(ctx, `
		INSERT INTO guilds (discord_id, name, admin_discord_id, anonymous, timezone, survey_hour, report_frequency, language)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (discord_id) DO UPDATE
		SET name = EXCLUDED.name, admin_discord_id = EXCLUDED.admin_discord_id,
		    active = TRUE, updated_at = now()
		RETURNING `+guildCols,
		g.DiscordID, g.Name, g.AdminDiscordID, s.Anonymous, s.Timezone, s.SurveyHour, string(s.ReportFrequency), s.Language))
}

func (r *GuildRepo) Get(ctx context.Context, guildID string) (domain.Guild, error) {
	return scanGuild(r.db.QueryRow(ctx, `SELECT `+guildCols+` FROM guilds WHERE discord_id = $1`, guildID))
}

func (r *GuildRepo) ListActive(ctx context.Context) ([]domain.Guild, error) {
	return r.list(ctx, `SELECT `+guildCols+` FROM guilds WHERE active ORDER BY id`)
}

func (r *GuildRepo) ListVisible(ctx context.Context, userID string, guildIDs []string) ([]domain.Guild, error) {
	if guildIDs == nil {
		guildIDs = []string{}
	}
	return r.list(ctx, `SELECT `+guildCols+` FROM guilds
		WHERE admin_discord_id = $1 OR discord_id = ANY($2) ORDER BY name, id`, userID, guildIDs)
}

func (r *GuildRepo) list(ctx context.Context, sql string, args ...any) ([]domain.Guild, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Guild
	for rows.Next() {
		g, err := scanGuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *GuildRepo) UpdateConfig(ctx context.Context, guildID, adminID string, s domain.GuildSettings) (domain.Guild, error) {
	return scanGuild(r.db.QueryRow(ctx, `
		UPDATE guilds SET admin_discord_id = $2, anonymous = $3, timezone = $4, survey_hour = $5,
		       report_frequency = $6, language = $7, updated_at = now()
		WHERE discord_id = $1
		RETURNING `+guildCols,
		guildID, adminID, s.Anonymous, s.Timezone, s.SurveyHour, string(s.ReportFrequency), s.Language))
}

func (r *GuildRepo) Deactivate(ctx context.Context, guildID string) error {
	tag, err := r.db.Exec(ctx, `UPDATE guilds SET active = FALSE, updated_at = now() WHERE discord_id = $1`, guildID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Members ---

type MemberRepo struct{ db *pgxpool.Pool }

func NewMemberRepo(db *pgxpool.Pool) *MemberRepo { return &MemberRepo{db: db} }

const memberCols = `g.discord_id, m.discord_id, m.username, m.active, m.created_at`

func scanMember(row pgx.Row) (domain.Member, error) {
	var m domain.Member
	err := row.Scan(&m.GuildDiscordID, &m.DiscordID, &m.Username, &m.Active, &m.CreatedAt)
	return m, notFound(err)
}

func (r *MemberRepo) Upsert(ctx context.Context, m domain.Member, setActive *bool) (domain.Member, error) {
	return scanMember(r.db.QueryRow(ctx, `
		WITH g AS (SELECT id, discord_id FROM guilds WHERE discord_id = $1),
		     m AS (
		         INSERT INTO members (guild_id, discord_id, username, active)
		         SELECT g.id, $2, $3, COALESCE($4::boolean, TRUE) FROM g
		         ON CONFLICT (guild_id, discord_id) DO UPDATE
		         SET username = EXCLUDED.username,
		             active = COALESCE($4::boolean, members.active),
		             updated_at = now()
		         RETURNING *
		     )
		SELECT `+memberCols+` FROM m JOIN g ON g.id = m.guild_id`,
		m.GuildDiscordID, m.DiscordID, m.Username, setActive))
}

func (r *MemberRepo) Get(ctx context.Context, guildID, userID string) (domain.Member, error) {
	return scanMember(r.db.QueryRow(ctx, `
		SELECT `+memberCols+` FROM members m JOIN guilds g ON g.id = m.guild_id
		WHERE g.discord_id = $1 AND m.discord_id = $2`, guildID, userID))
}

func (r *MemberRepo) ListActive(ctx context.Context, guildID string) ([]domain.Member, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+memberCols+` FROM members m JOIN guilds g ON g.id = m.guild_id
		WHERE g.discord_id = $1 AND m.active ORDER BY m.id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MemberRepo) Deactivate(ctx context.Context, guildID, userID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE members m SET active = FALSE, updated_at = now()
		FROM guilds g
		WHERE g.id = m.guild_id AND g.discord_id = $1 AND m.discord_id = $2`, guildID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Moods ---

type MoodRepo struct{ db *pgxpool.Pool }

func NewMoodRepo(db *pgxpool.Pool) *MoodRepo { return &MoodRepo{db: db} }

const moodCols = `g.discord_id, m.discord_id, e.day, e.mood, e.message, e.created_at, e.updated_at`

func scanMood(row pgx.Row) (domain.MoodEntry, error) {
	var e domain.MoodEntry
	var mood int16
	err := row.Scan(&e.GuildDiscordID, &e.MemberDiscordID, &e.Day, &mood, &e.Message, &e.CreatedAt, &e.UpdatedAt)
	e.Mood = domain.Mood(mood)
	e.Day = domain.TruncateDay(e.Day)
	return e, notFound(err)
}

func (r *MoodRepo) Upsert(ctx context.Context, e domain.MoodEntry) (domain.MoodEntry, error) {
	// The member lookup is scoped to the guild, so an entry can never be attached to
	// another guild's member.
	return scanMood(r.db.QueryRow(ctx, `
		WITH t AS (
		         SELECT g.id AS guild_id, g.discord_id AS guild_discord_id, m.id AS member_id, m.discord_id AS member_discord_id
		         FROM members m JOIN guilds g ON g.id = m.guild_id
		         WHERE g.discord_id = $1 AND m.discord_id = $2
		     ),
		     e AS (
		         INSERT INTO mood_entries (guild_id, member_id, day, mood, message)
		         SELECT t.guild_id, t.member_id, $3, $4, $5 FROM t
		         ON CONFLICT (member_id, day) DO UPDATE
		         SET mood = EXCLUDED.mood, message = EXCLUDED.message, updated_at = now()
		         RETURNING *
		     )
		SELECT t.guild_discord_id, t.member_discord_id, e.day, e.mood, e.message, e.created_at, e.updated_at
		FROM e JOIN t ON t.member_id = e.member_id`,
		e.GuildDiscordID, e.MemberDiscordID, e.Day, int16(e.Mood), e.Message))
}

func (r *MoodRepo) ListBetween(ctx context.Context, guildID string, from, to time.Time) ([]domain.MoodEntry, error) {
	return r.list(ctx, `
		SELECT `+moodCols+`
		FROM mood_entries e
		JOIN guilds g ON g.id = e.guild_id
		JOIN members m ON m.id = e.member_id
		WHERE g.discord_id = $1 AND e.day BETWEEN $2 AND $3
		ORDER BY e.day, e.id`, guildID, from, to)
}

func (r *MoodRepo) ListForUser(ctx context.Context, userID string, from, to time.Time) ([]domain.MoodEntry, error) {
	return r.list(ctx, `
		SELECT `+moodCols+`
		FROM mood_entries e
		JOIN guilds g ON g.id = e.guild_id
		JOIN members m ON m.id = e.member_id
		WHERE m.discord_id = $1 AND e.day BETWEEN $2 AND $3
		ORDER BY e.day, e.id`, userID, from, to)
}

func (r *MoodRepo) list(ctx context.Context, sql string, args ...any) ([]domain.MoodEntry, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MoodEntry
	for rows.Next() {
		e, err := scanMood(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Dispatches ---

type DispatchRepo struct{ db *pgxpool.Pool }

func NewDispatchRepo(db *pgxpool.Pool) *DispatchRepo { return &DispatchRepo{db: db} }

func (r *DispatchRepo) Claim(ctx context.Context, guildID string, kind domain.DispatchKind, period time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO dispatches (guild_id, kind, period)
		SELECT id, $2, $3 FROM guilds WHERE discord_id = $1
		ON CONFLICT DO NOTHING`, guildID, string(kind), period)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
