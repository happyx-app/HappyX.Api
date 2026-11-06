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
// are a member; one set by a server's admin is scoped to that guild like any tenant row.
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

func (r *VacationRepo) Set(ctx context.Context, v domain.Vacation, notify bool) (domain.Vacation, error) {
	var guild any // NULL for the person's own
	if v.GuildDiscordID != "" {
		var id int64
		err := r.db.QueryRow(ctx, `SELECT id FROM guilds WHERE discord_id = $1`, v.GuildDiscordID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Vacation{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Vacation{}, err
		}
		guild = id
	}
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO vacations (user_discord_id, guild_id, starts_on, ends_on, set_by_discord_id, notify_pending)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_discord_id, COALESCE(guild_id, 0)) DO UPDATE
		SET starts_on = EXCLUDED.starts_on, ends_on = EXCLUDED.ends_on,
		    set_by_discord_id = EXCLUDED.set_by_discord_id, notify_pending = EXCLUDED.notify_pending,
		    created_at = now()
		RETURNING id`,
		v.UserDiscordID, guild, v.From, v.To, v.SetByDiscordID, notify).Scan(&id)
	if err != nil {
		return domain.Vacation{}, err
	}
	return scanVacation(r.db.QueryRow(ctx, `SELECT `+vacationCols+vacationFrom+`WHERE v.id = $1`, id))
}

func (r *VacationRepo) Delete(ctx context.Context, userID, guildID string) error {
	var tag interface{ RowsAffected() int64 }
	var err error
	if guildID == "" {
		tag, err = r.db.Exec(ctx, `DELETE FROM vacations WHERE user_discord_id = $1 AND guild_id IS NULL`, userID)
	} else {
		tag, err = r.db.Exec(ctx, `
			DELETE FROM vacations WHERE user_discord_id = $1
			AND guild_id = (SELECT id FROM guilds WHERE discord_id = $2)`, userID, guildID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *VacationRepo) ForUser(ctx context.Context, userID string, since time.Time) ([]domain.Vacation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.user_discord_id = $1 AND v.ends_on >= $2
		ORDER BY v.guild_id NULLS FIRST, vg.name, v.id`, userID, since)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) { return scanVacation(row) })
}

func (r *VacationRepo) ForGuild(ctx context.Context, guildID string, since time.Time) ([]domain.Vacation, error) {
	// The members' own vacations count here too; a member who left the server is no longer listed.
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+`, m.username`+vacationFrom+`
		JOIN guilds g ON g.discord_id = $1
		JOIN members m ON m.guild_id = g.id AND m.discord_id = v.user_discord_id AND m.in_server
		WHERE (v.guild_id IS NULL OR v.guild_id = g.id) AND v.ends_on >= $2
		ORDER BY v.starts_on, lower(m.username), v.guild_id NULLS FIRST`, guildID, since)
	return collect(rows, err, func(row pgx.Row) (domain.Vacation, error) {
		var name string
		v, err := scanVacation(row, &name)
		v.Username = name
		return v, err
	})
}

func (r *VacationRepo) PendingNotices(ctx context.Context) ([]domain.Vacation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+vacationCols+vacationFrom+`
		WHERE v.notify_pending AND vg.active ORDER BY v.id LIMIT 100`)
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
