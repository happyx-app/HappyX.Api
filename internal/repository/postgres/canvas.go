package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Team canvas ---

type CanvasRepo struct{ db *pgxpool.Pool }

func NewCanvasRepo(db *pgxpool.Pool) *CanvasRepo { return &CanvasRepo{db: db} }

const canvasCols = `s.id, g.discord_id, m.discord_id, m.username, s.drawing_id, s.color, s.anchor_x, s.anchor_y, s.points, s.created_at`

func scanCanvasStroke(row pgx.Row) (domain.CanvasStroke, error) {
	var s domain.CanvasStroke
	var ax, ay float32
	var color int16
	var points []byte
	err := row.Scan(&s.ID, &s.GuildDiscordID, &s.AuthorDiscordID, &s.AuthorUsername, &s.DrawingID, &color, &ax, &ay, &points, &s.CreatedAt)
	s.Color, s.AnchorX, s.AnchorY, s.Points = int(color), float64(ax), float64(ay), domain.DecodeCanvasPoints(points)
	return s, notFound(err)
}

func (r *CanvasRepo) List(ctx context.Context, guildID string) ([]domain.CanvasStroke, int64, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var version int64
	if err := tx.QueryRow(ctx, `SELECT canvas_version FROM guilds WHERE discord_id = $1`, guildID).Scan(&version); err != nil {
		return nil, 0, notFound(err)
	}
	rows, qerr := tx.Query(ctx, `
		SELECT `+canvasCols+`
		FROM canvas_strokes s
		JOIN guilds g ON g.id = s.guild_id
		JOIN members m ON m.id = s.member_id
		WHERE g.discord_id = $1
		ORDER BY s.id`, guildID)
	out, err := collect(rows, qerr, scanCanvasStroke)
	return out, version, err
}

func (r *CanvasRepo) Version(ctx context.Context, guildID string) (int64, error) {
	var v int64
	err := r.db.QueryRow(ctx, `SELECT canvas_version FROM guilds WHERE discord_id = $1`, guildID).Scan(&v)
	return v, notFound(err)
}

// bump raises the guild's canvas version in tx and returns the new one.
func bumpCanvas(ctx context.Context, tx pgx.Tx, guildID string) (int64, error) {
	var v int64
	err := tx.QueryRow(ctx, `UPDATE guilds SET canvas_version = canvas_version + 1 WHERE discord_id = $1 RETURNING canvas_version`, guildID).Scan(&v)
	return v, notFound(err)
}

func (r *CanvasRepo) Add(ctx context.Context, s domain.CanvasStroke, since time.Time) (domain.CanvasStroke, int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Raising the version first locks the guild's row, so strokes for one guild are added one at
	// a time and the counts below can't be raced past.
	v, err := bumpCanvas(ctx, tx, s.GuildDiscordID)
	if err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	var total, recent int
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE m.discord_id = $2 AND s.created_at >= $3)
		FROM canvas_strokes s
		JOIN guilds g ON g.id = s.guild_id
		JOIN members m ON m.id = s.member_id
		WHERE g.discord_id = $1`, s.GuildDiscordID, s.AuthorDiscordID, since).Scan(&total, &recent); err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	switch {
	case recent >= domain.MaxCanvasStrokesPerMinute:
		return domain.CanvasStroke{}, 0, domain.ErrTooMany
	case total >= domain.MaxCanvasStrokes:
		return domain.CanvasStroke{}, 0, domain.ErrCanvasFull
	}
	out, err := scanCanvasStroke(tx.QueryRow(ctx, `
		WITH s AS (
		         INSERT INTO canvas_strokes (guild_id, member_id, drawing_id, color, anchor_x, anchor_y, points)
		         SELECT g.id, m.id, $3, $4, $5, $6, $7
		         FROM guilds g JOIN members m ON m.guild_id = g.id AND m.discord_id = $2
		         WHERE g.discord_id = $1
		         RETURNING *
		     )
		SELECT `+canvasCols+`
		FROM s JOIN guilds g ON g.id = s.guild_id JOIN members m ON m.id = s.member_id`,
		s.GuildDiscordID, s.AuthorDiscordID, s.DrawingID, int16(s.Color), float32(s.AnchorX), float32(s.AnchorY), domain.EncodeCanvasPoints(s.Points)))
	if err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	return out, v, tx.Commit(ctx)
}

func (r *CanvasRepo) DeleteDrawing(ctx context.Context, guildID, drawingID, authorID string) (int, int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		DELETE FROM canvas_strokes s
		USING guilds g, members m
		WHERE g.id = s.guild_id AND m.id = s.member_id
		  AND g.discord_id = $1 AND s.drawing_id = $2 AND ($3 = '' OR m.discord_id = $3)`, guildID, drawingID, authorID)
	if err != nil {
		return 0, 0, err
	}
	n := int(tag.RowsAffected())
	if n == 0 {
		return 0, 0, nil
	}
	v, err := bumpCanvas(ctx, tx, guildID)
	if err != nil {
		return 0, 0, err
	}
	return n, v, tx.Commit(ctx)
}

func (r *CanvasRepo) Clear(ctx context.Context, guildID string) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		DELETE FROM canvas_strokes s USING guilds g
		WHERE g.id = s.guild_id AND g.discord_id = $1`, guildID); err != nil {
		return 0, err
	}
	v, err := bumpCanvas(ctx, tx, guildID)
	if err != nil {
		return 0, err
	}
	return v, tx.Commit(ctx)
}
