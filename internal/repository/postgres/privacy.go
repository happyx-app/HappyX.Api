package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Personal data ---

// PrivacyRepo reads and erases data by person or by guild, for exports and erasure requests.
// Person queries filter on the member's Discord ID and join each row to its own guild, so every
// row is reported under the guild it belongs to.
type PrivacyRepo struct{ db *pgxpool.Pool }

func NewPrivacyRepo(db *pgxpool.Pool) *PrivacyRepo { return &PrivacyRepo{db: db} }

func collect[T any](rows pgx.Rows, err error, scan func(pgx.Row) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (T, error) { return scan(r) })
	if out == nil {
		out = []T{}
	}
	return out, err
}

func (r *PrivacyRepo) UserData(ctx context.Context, userID string) (domain.UserData, error) {
	d := domain.UserData{DiscordID: userID, Guilds: []domain.MemberData{}}
	var err error
	rows, qerr := r.db.Query(ctx, `SELECT `+guildCols+` FROM guilds WHERE admin_discord_id = $1 ORDER BY name, id`, userID)
	if d.ReportsFor, err = collect(rows, qerr, scanGuild); err != nil {
		return d, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT `+memberCols+` FROM members m JOIN guilds g ON g.id = m.guild_id
		WHERE m.discord_id = $1 ORDER BY g.name, g.id`, userID)
	members, err := collect(rows, qerr, scanMember)
	if err != nil {
		return d, err
	}
	for _, m := range members {
		md, err := r.memberData(ctx, m)
		if err != nil {
			return d, err
		}
		d.Guilds = append(d.Guilds, md)
	}
	return d, nil
}

func (r *PrivacyRepo) memberData(ctx context.Context, m domain.Member) (domain.MemberData, error) {
	g, u := m.GuildDiscordID, m.DiscordID
	md := domain.MemberData{Member: m}
	var err error
	if md.Guild, err = scanGuild(r.db.QueryRow(ctx, `SELECT `+guildCols+` FROM guilds WHERE discord_id = $1`, g)); err != nil {
		return md, err
	}
	rows, qerr := r.db.Query(ctx, `
		SELECT `+moodCols+`
		FROM mood_entries e
		JOIN guilds g ON g.id = e.guild_id
		JOIN members m ON m.id = e.member_id AND m.guild_id = g.id
		WHERE g.discord_id = $1 AND m.discord_id = $2
		ORDER BY e.day`, g, u)
	if md.Moods, err = collect(rows, qerr, scanMood); err != nil {
		return md, err
	}
	kudos := func(side string) ([]domain.Kudos, error) {
		rows, err := r.db.Query(ctx, `
			SELECT `+kudosCols+`
			FROM kudos k
			JOIN guilds g ON g.id = k.guild_id
			JOIN members gm ON gm.id = k.giver_member_id
			JOIN members rm ON rm.id = k.receiver_member_id
			WHERE g.discord_id = $1 AND `+side+`.discord_id = $2
			ORDER BY k.created_at, k.id`, g, u)
		return collect(rows, err, scanKudos)
	}
	if md.KudosGiven, err = kudos("gm"); err != nil {
		return md, err
	}
	if md.KudosReceived, err = kudos("rm"); err != nil {
		return md, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT rd.week, rd.question, a.answer, m.discord_id, m.username
		FROM game_answers a
		JOIN game_rounds rd ON rd.id = a.round_id
		JOIN guilds g ON g.id = rd.guild_id
		JOIN members m ON m.id = a.member_id AND m.guild_id = g.id
		WHERE g.discord_id = $1 AND m.discord_id = $2
		ORDER BY rd.week`, g, u)
	if md.GameAnswers, err = collect(rows, qerr, scanRoundAnswer); err != nil {
		return md, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT rd.week, rd.question, a.answer, tm.username
		FROM game_guesses gs
		JOIN game_answers a ON a.id = gs.answer_id
		JOIN game_rounds rd ON rd.id = a.round_id
		JOIN guilds g ON g.id = rd.guild_id
		JOIN members gm ON gm.id = gs.guesser_member_id AND gm.guild_id = g.id
		JOIN members tm ON tm.id = gs.guessed_member_id AND tm.guild_id = g.id
		WHERE g.discord_id = $1 AND gm.discord_id = $2
		ORDER BY rd.week, gs.created_at, a.id`, g, u)
	if md.GameGuesses, err = collect(rows, qerr, func(row pgx.Row) (domain.MadeGuess, error) {
		var mg domain.MadeGuess
		err := row.Scan(&mg.Week, &mg.Question, &mg.Answer, &mg.GuessedUsername)
		mg.Week = domain.TruncateDay(mg.Week)
		return mg, err
	}); err != nil {
		return md, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT `+gameProposalCols+`
		FROM game_proposals p
		JOIN guilds g ON g.id = p.guild_id
		JOIN members m ON m.id = p.member_id AND m.guild_id = g.id
		WHERE g.discord_id = $1 AND m.discord_id = $2
		ORDER BY p.week`, g, u)
	md.GameProposals, err = collect(rows, qerr, scanGameProposal)
	return md, err
}

func scanRoundAnswer(row pgx.Row) (domain.RoundAnswer, error) {
	var a domain.RoundAnswer
	err := row.Scan(&a.Week, &a.Question, &a.Text, &a.MemberDiscordID, &a.Username)
	a.Week = domain.TruncateDay(a.Week)
	return a, err
}

func (r *PrivacyRepo) EraseUser(ctx context.Context, userID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	const mine = `(SELECT id FROM members WHERE discord_id = $1)`
	for _, sql := range []string{
		`DELETE FROM mood_entries WHERE member_id IN ` + mine,
		`DELETE FROM kudos WHERE giver_member_id IN ` + mine + ` OR receiver_member_id IN ` + mine,
		// Their guesses, and everyone's guesses that they wrote an answer.
		`DELETE FROM game_guesses WHERE guesser_member_id IN ` + mine + ` OR guessed_member_id IN ` + mine,
		// Guesses on their answers go with the answers.
		`DELETE FROM game_answers WHERE member_id IN ` + mine,
		`DELETE FROM game_proposals WHERE member_id IN ` + mine,
		// A question they proposed stays in its round, without their name.
		`UPDATE game_rounds SET proposer_member_id = NULL WHERE proposer_member_id IN ` + mine,
		`UPDATE members SET active = FALSE, username = '', updated_at = now() WHERE discord_id = $1`,
	} {
		if _, err := tx.Exec(ctx, sql, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PrivacyRepo) GuildData(ctx context.Context, guildID string) (domain.GuildData, error) {
	var d domain.GuildData
	var err error
	if d.Guild, err = scanGuild(r.db.QueryRow(ctx, `SELECT `+guildCols+` FROM guilds WHERE discord_id = $1`, guildID)); err != nil {
		return d, err
	}
	rows, qerr := r.db.Query(ctx, `
		SELECT `+memberCols+` FROM members m JOIN guilds g ON g.id = m.guild_id
		WHERE g.discord_id = $1 ORDER BY lower(m.username), m.discord_id`, guildID)
	if d.Members, err = collect(rows, qerr, scanMember); err != nil {
		return d, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT `+moodCols+`
		FROM mood_entries e
		JOIN guilds g ON g.id = e.guild_id
		JOIN members m ON m.id = e.member_id
		WHERE g.discord_id = $1
		ORDER BY e.day, e.id`, guildID)
	if d.Moods, err = collect(rows, qerr, scanMood); err != nil {
		return d, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT `+kudosCols+`
		FROM kudos k
		JOIN guilds g ON g.id = k.guild_id
		JOIN members gm ON gm.id = k.giver_member_id
		JOIN members rm ON rm.id = k.receiver_member_id
		WHERE g.discord_id = $1
		ORDER BY k.created_at, k.id`, guildID)
	if d.Kudos, err = collect(rows, qerr, scanKudos); err != nil {
		return d, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT `+gameRoundCols+`
		FROM game_rounds rd JOIN guilds g ON g.id = rd.guild_id
		LEFT JOIN members m ON m.id = rd.proposer_member_id
		WHERE g.discord_id = $1
		ORDER BY rd.week`, guildID)
	if d.Rounds, err = collect(rows, qerr, scanGameRound); err != nil {
		return d, err
	}
	rows, qerr = r.db.Query(ctx, `
		SELECT rd.week, rd.question, a.answer, m.discord_id, m.username
		FROM game_answers a
		JOIN game_rounds rd ON rd.id = a.round_id
		JOIN guilds g ON g.id = rd.guild_id
		JOIN members m ON m.id = a.member_id AND m.guild_id = g.id
		WHERE g.discord_id = $1 AND m.active
		ORDER BY rd.week, a.ref`, guildID)
	d.Answers, err = collect(rows, qerr, scanRoundAnswer)
	return d, err
}
