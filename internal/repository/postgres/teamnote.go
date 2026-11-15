package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Team notes ---

// TeamNoteRepo stores what admins told the team after a report. Every query is scoped to one
// guild by its Discord ID.
type TeamNoteRepo struct{ db *pgxpool.Pool }

func NewTeamNoteRepo(db *pgxpool.Pool) *TeamNoteRepo { return &TeamNoteRepo{db: db} }

const teamNoteCols = `n.id, g.discord_id, m.discord_id, m.username, n.period_from, n.period_to, n.body,
	n.channel_id, n.message_id, n.posted_on, n.created_at, n.updated_at`

const teamNoteFrom = ` FROM team_notes n JOIN guilds g ON g.id = n.guild_id JOIN members m ON m.id = n.author_member_id `

func scanTeamNote(row pgx.Row) (domain.TeamNote, error) {
	var n domain.TeamNote
	err := row.Scan(&n.ID, &n.GuildDiscordID, &n.AuthorDiscordID, &n.AuthorUsername, &n.From, &n.To, &n.Body,
		&n.ChannelID, &n.MessageID, &n.PostedOn, &n.CreatedAt, &n.UpdatedAt)
	n.From, n.To, n.PostedOn = domain.TruncateDay(n.From), domain.TruncateDay(n.To), domain.TruncateDay(n.PostedOn)
	return n, notFound(err)
}

func (r *TeamNoteRepo) Create(ctx context.Context, n domain.TeamNote) (domain.TeamNote, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO team_notes (guild_id, author_member_id, period_from, period_to, body, channel_id, message_id, posted_on)
		SELECT g.id, m.id, $3, $4, $5, $6, $7, $8
		FROM guilds g JOIN members m ON m.guild_id = g.id
		WHERE g.discord_id = $1 AND m.discord_id = $2
		ON CONFLICT (guild_id, period_from, period_to) DO NOTHING
		RETURNING id`,
		n.GuildDiscordID, n.AuthorDiscordID, n.From, n.To, n.Body, n.ChannelID, n.MessageID, n.PostedOn).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing inserted: the period already has a note, or the author isn't a member.
		if _, perr := r.ForPeriod(ctx, n.GuildDiscordID, n.From, n.To); perr == nil {
			return domain.TeamNote{}, domain.ErrTeamNoteTold
		}
	}
	if err != nil {
		return domain.TeamNote{}, notFound(err)
	}
	return r.Get(ctx, n.GuildDiscordID, id)
}

func (r *TeamNoteRepo) Get(ctx context.Context, guildID string, id int64) (domain.TeamNote, error) {
	return scanTeamNote(r.db.QueryRow(ctx, `SELECT `+teamNoteCols+teamNoteFrom+`WHERE g.discord_id = $1 AND n.id = $2`, guildID, id))
}

func (r *TeamNoteRepo) UpdateBody(ctx context.Context, guildID string, id int64, body string) (domain.TeamNote, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE team_notes n SET body = $3, updated_at = now()
		FROM guilds g WHERE g.id = n.guild_id AND g.discord_id = $1 AND n.id = $2`, guildID, id, body)
	if err != nil {
		return domain.TeamNote{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.TeamNote{}, domain.ErrNotFound
	}
	return r.Get(ctx, guildID, id)
}

func (r *TeamNoteRepo) Delete(ctx context.Context, guildID string, id int64) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM team_notes n USING guilds g
		WHERE g.id = n.guild_id AND g.discord_id = $1 AND n.id = $2`, guildID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TeamNoteRepo) Latest(ctx context.Context, guildID string, since, until time.Time) (domain.TeamNote, error) {
	return scanTeamNote(r.db.QueryRow(ctx, `SELECT `+teamNoteCols+teamNoteFrom+`
		WHERE g.discord_id = $1 AND n.posted_on >= $2 AND n.posted_on < $3
		ORDER BY n.created_at DESC, n.id DESC LIMIT 1`, guildID, since, until))
}

func (r *TeamNoteRepo) ForPeriod(ctx context.Context, guildID string, from, to time.Time) (domain.TeamNote, error) {
	return scanTeamNote(r.db.QueryRow(ctx, `SELECT `+teamNoteCols+teamNoteFrom+`
		WHERE g.discord_id = $1 AND n.period_from = $2 AND n.period_to = $3
		ORDER BY n.created_at DESC, n.id DESC LIMIT 1`, guildID, from, to))
}
