package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Vacations ---

// VacationRepo stores vacations. A person's own vacation has no guild and applies wherever they
// are a member; one set by a server's admin is scoped to that guild like any tenant row. A
// replaced or removed vacation stays, archived, for the days before keepFrom.
type VacationRepo struct{ db *pgxpool.Pool }

func NewVacationRepo(db *pgxpool.Pool) *VacationRepo { return &VacationRepo{db: db} }

const vacationCols = `v.id, v.user_discord_id, COALESCE(vg.discord_id, ''), COALESCE(vg.name, ''), v.starts_on, v.ends_on, v.set_by_discord_id, v.created_at`

const vacationFrom = ` FROM vacations v LEFT JOIN guilds vg ON vg.id = v.guild_id `

func scanVacation(row pgx.Row, extra ...any) (domain.Vacation, error) {
	var v domain.Vacation
	err := row.Scan(append([]any{&v.ID, &v.UserDiscordID, &v.GuildDiscordID, &v.GuildName, &v.From, &v.To, &v.SetByDiscordID, &v.CreatedAt}, extra...)...)
	v.From, v.To = domain.TruncateDay(v.From), domain.TruncateDay(v.To)
	return v, err
}

// guildRef resolves the guild of a vacation's scope: NULL for the person's own.
func guildRef(ctx context.Context, q pgx.Tx, guildID string) (any, error) {
	if guildID == "" {
		return nil, nil
	}
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM guilds WHERE discord_id = $1`, guildID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return id, err
}

// retire takes the person's current vacation of the scope out of the way: the days before
// keepFrom stay, archived, and the rest goes. It reports whether there was one.
func retire(ctx context.Context, tx pgx.Tx, userID string, guild any, keepFrom time.Time) (bool, error) {
	var id int64
	var starts time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, starts_on FROM vacations
		WHERE user_discord_id = $1 AND guild_id IS NOT DISTINCT FROM $2 AND NOT archived
		FOR UPDATE`, userID, guild).Scan(&id, &starts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if domain.TruncateDay(starts).Before(keepFrom) {
		_, err = tx.Exec(ctx, `
			UPDATE vacations SET archived = TRUE, notify_pending = FALSE, ends_on = LEAST(ends_on, $2::date - 1)
			WHERE id = $1`, id, keepFrom)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM vacations WHERE id = $1`, id)
	}
	return true, err
}

func (r *VacationRepo) Set(ctx context.Context, v domain.Vacation, notify bool, keepFrom time.Time) (domain.Vacation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Vacation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	guild, err := guildRef(ctx, tx, v.GuildDiscordID)
	if err != nil {
		return domain.Vacation{}, err
	}
	if _, err := retire(ctx, tx, v.UserDiscordID, guild, domain.TruncateDay(keepFrom)); err != nil {
		return domain.Vacation{}, err
	}
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO vacations (user_discord_id, guild_id, starts_on, ends_on, set_by_discord_id, notify_pending)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		v.UserDiscordID, guild, v.From, v.To, v.SetByDiscordID, notify).Scan(&id)
	if err != nil {
		return domain.Vacation{}, err
	}
	saved, err := scanVacation(tx.QueryRow(ctx, `SELECT `+vacationCols+vacationFrom+`WHERE v.id = $1`, id))
	if err != nil {
		return domain.Vacation{}, err
	}
	return saved, tx.Commit(ctx)
}

func (r *VacationRepo) Delete(ctx context.Context, userID, guildID string, keepFrom time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	guild, err := guildRef(ctx, tx, guildID)
	if err != nil {
		return err
	}
	found, err := retire(ctx, tx, userID, guild, domain.TruncateDay(keepFrom))
	if err != nil {
		return err
	}
	if !found {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *VacationRepo) EndCovering(ctx context.Context, userID, guildID string, day, last time.Time) ([]domain.Vacation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	rows, err := tx.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.user_discord_id = $1 AND NOT v.archived
		AND (v.guild_id IS NULL OR v.guild_id = (SELECT id FROM guilds WHERE discord_id = $2))
		AND v.starts_on <= $3 AND v.ends_on >= $3
		ORDER BY v.guild_id NULLS FIRST
		FOR UPDATE OF v`, userID, guildID, day)
	ended, err := collect(rows, err, func(row pgx.Row) (domain.Vacation, error) { return scanVacation(row) })
	if err != nil {
		return nil, err
	}
	for _, v := range ended {
		// The days already gone stay vacation, so they keep not counting against anyone.
		if v.From.After(domain.TruncateDay(last)) {
			_, err = tx.Exec(ctx, `DELETE FROM vacations WHERE id = $1`, v.ID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE vacations SET ends_on = $2 WHERE id = $1`, v.ID, domain.TruncateDay(last))
		}
		if err != nil {
			return nil, err
		}
	}
	return ended, tx.Commit(ctx)
}

func (r *VacationRepo) ForUser(ctx context.Context, userID string, since time.Time) ([]domain.Vacation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.user_discord_id = $1 AND v.ends_on >= $2 AND NOT v.archived
		ORDER BY v.guild_id NULLS FIRST, vg.name, v.id`, userID, since)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) { return scanVacation(row) })
}

func (r *VacationRepo) History(ctx context.Context, userID string, since time.Time) ([]domain.Vacation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.user_discord_id = $1 AND v.ends_on >= $2
		ORDER BY v.starts_on, v.id`, userID, since)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) { return scanVacation(row) })
}

func (r *VacationRepo) ForGuild(ctx context.Context, guildID string, since time.Time) ([]domain.Vacation, error) {
	// The members' own vacations count here too; a member who left the server is no longer listed.
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+`, m.username, `+displayName("m")+vacationFrom+`
		JOIN guilds g ON g.discord_id = $1
		JOIN members m ON m.guild_id = g.id AND m.discord_id = v.user_discord_id AND m.in_server
		WHERE (v.guild_id IS NULL OR v.guild_id = g.id) AND v.ends_on >= $2 AND NOT v.archived
		ORDER BY v.starts_on, lower(`+displayName("m")+`), v.guild_id NULLS FIRST`, guildID, since)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) {
		var username, name string
		v, err := scanVacation(row, &username, &name)
		v.Username, v.DisplayName = username, name
		return v, err
	})
}

func (r *VacationRepo) PendingNotices(ctx context.Context) ([]domain.Vacation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.notify_pending AND NOT v.archived AND vg.active ORDER BY v.id LIMIT 100`)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) { return scanVacation(row) })
}

func (r *VacationRepo) DoneNotice(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `UPDATE vacations SET notify_pending = FALSE WHERE id = $1 AND notify_pending`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *VacationRepo) DeleteEndedBefore(ctx context.Context, before time.Time) (int, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM vacations WHERE ends_on < $1`, before)
	return int(tag.RowsAffected()), err
}
