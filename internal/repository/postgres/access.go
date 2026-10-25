package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// --- Access ---

type AccessRepo struct{ db *pgxpool.Pool }

func NewAccessRepo(db *pgxpool.Pool) *AccessRepo { return &AccessRepo{db: db} }

func (r *AccessRepo) ReplaceRoles(ctx context.Context, guildID string, roles []domain.Role) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	var gid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM guilds WHERE discord_id = $1 FOR KEY SHARE`, guildID).Scan(&gid); err != nil {
		return notFound(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM guild_roles WHERE guild_id = $1`, gid); err != nil {
		return err
	}
	ids := make([]string, 0, len(roles))
	for _, ro := range roles {
		if _, err := tx.Exec(ctx, `
			INSERT INTO guild_roles (guild_id, discord_id, name, color, position) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (guild_id, discord_id) DO UPDATE SET name = EXCLUDED.name, color = EXCLUDED.color, position = EXCLUDED.position`,
			gid, ro.DiscordID, ro.Name, ro.Color, ro.Position); err != nil {
			return err
		}
		ids = append(ids, ro.DiscordID)
	}
	// A grant to a deleted role would come back to life if Discord ever reused the ID.
	if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE guild_id = $1 AND kind = 'role' AND NOT target_id = ANY($2)`, gid, ids); err != nil {
		return err
	}
	// The same goes for the survey and game roles. A list left with none of its roles goes back
	// to everyone: "only" nobody would silently stop the survey or the game for the whole server.
	for _, col := range []string{"survey", "game"} {
		if _, err := tx.Exec(ctx, strings.NewReplacer("{col}", col).Replace(`
			UPDATE guilds SET
			       {col}_role_ids = kept,
			       {col}_roles_mode = CASE WHEN cardinality(kept) = 0 THEN 'all' ELSE {col}_roles_mode END
			FROM (SELECT ARRAY(SELECT r FROM unnest({col}_role_ids) WITH ORDINALITY AS t(r, n) WHERE r = ANY($2) ORDER BY n) AS kept
			      FROM guilds WHERE id = $1) k
			WHERE id = $1 AND NOT {col}_role_ids <@ $2`), gid, ids); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *AccessRepo) ListRoles(ctx context.Context, guildID string) ([]domain.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ro.discord_id, ro.name, ro.color, ro.position
		FROM guild_roles ro JOIN guilds g ON g.id = ro.guild_id
		WHERE g.discord_id = $1 ORDER BY ro.position DESC, ro.discord_id`, guildID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Role, error) {
		var ro domain.Role
		err := row.Scan(&ro.DiscordID, &ro.Name, &ro.Color, &ro.Position)
		return ro, err
	})
}

func (r *AccessRepo) ListGrants(ctx context.Context, guildID string) ([]domain.AccessGrant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.kind, a.target_id, a.level
		FROM access_grants a JOIN guilds g ON g.id = a.guild_id
		WHERE g.discord_id = $1 ORDER BY a.created_at, a.kind, a.target_id`, guildID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AccessGrant, error) {
		var gr domain.AccessGrant
		var kind, level string
		if err := row.Scan(&kind, &gr.TargetID, &level); err != nil {
			return gr, err
		}
		gr.Kind = domain.GrantKind(kind)
		gr.Level, err = domain.ParseGrantLevel(level)
		return gr, err
	})
}

func (r *AccessRepo) SetAccess(ctx context.Context, guildID, adminID string, grants []domain.AccessGrant) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	var gid int64
	if err := tx.QueryRow(ctx, `UPDATE guilds SET admin_discord_id = $2, updated_at = now() WHERE discord_id = $1 RETURNING id`,
		guildID, adminID).Scan(&gid); err != nil {
		return notFound(err)
	}
	// Grants that stay keep their created_at, so the list keeps its order.
	keep := make([]string, 0, len(grants))
	for _, gr := range grants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO access_grants (guild_id, kind, target_id, level) VALUES ($1, $2, $3, $4)
			ON CONFLICT (guild_id, kind, target_id) DO UPDATE SET level = EXCLUDED.level`,
			gid, string(gr.Kind), gr.TargetID, gr.Level.String()); err != nil {
			return err
		}
		keep = append(keep, string(gr.Kind)+":"+gr.TargetID)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE guild_id = $1 AND NOT (kind || ':' || target_id) = ANY($2)`, gid, keep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
