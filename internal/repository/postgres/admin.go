package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// AdminRepo reads the counts behind the owner's admin dashboard. Its queries never select a
// mood, a message, a username or any other text a member wrote.
type AdminRepo struct{ db *pgxpool.Pool }

func NewAdminRepo(db *pgxpool.Pool) *AdminRepo { return &AdminRepo{db: db} }

// answeredSQL counts answers to surveys whose recipients the bot reported, so the response rate
// compares like with like.
const answeredSQL = `JOIN dispatches d ON d.guild_id = e.guild_id AND d.kind = 'survey' AND d.period = e.day AND d.recipients IS NOT NULL`

func (r *AdminRepo) Overview(ctx context.Context, now time.Time) (domain.AdminOverview, error) {
	var o domain.AdminOverview
	today := domain.TruncateDay(now)

	// A guild deleted from the dashboard leaves too; one deleted after the bot left was already
	// counted when it left.
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE g.active),
		       count(*) FILTER (WHERE g.active AND g.created_at < $2::timestamptz - interval '14 days'
		                        AND NOT EXISTS (SELECT 1 FROM mood_entries e WHERE e.guild_id = g.id AND e.day >= $1::date - 14)),
		       count(*) FILTER (WHERE NOT g.active),
		       count(*) FILTER (WHERE g.created_at >= $2::timestamptz - interval '7 days'),
		       count(*) FILTER (WHERE NOT g.active AND g.left_at >= $2::timestamptz - interval '7 days')
		         + (SELECT count(*) FROM guild_deletions WHERE reason = 'dashboard' AND deleted_at >= $2::timestamptz - interval '7 days'),
		       (SELECT count(*) FROM guild_deletions)
		FROM guilds g`, today, now).Scan(&o.Guilds.WithBot, &o.Guilds.Idle, &o.Guilds.Left, &o.Guilds.NewWeek, &o.Guilds.LeftWeek, &o.Guilds.Deleted)
	if err != nil {
		return o, err
	}

	err = r.db.QueryRow(ctx, `
		SELECT count(DISTINCT m.discord_id) FILTER (WHERE m.in_server),
		       count(*) FILTER (WHERE m.in_server AND NOT m.active)
		FROM members m JOIN guilds g ON g.id = m.guild_id AND g.active`).Scan(&o.People.Members, &o.People.OptedOut)
	if err != nil {
		return o, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT count(DISTINCT m.discord_id) FILTER (WHERE e.day = $1),
		       count(DISTINCT m.discord_id) FILTER (WHERE e.day > $1::date - 7),
		       count(DISTINCT m.discord_id)
		FROM mood_entries e JOIN members m ON m.id = e.member_id
		WHERE e.day > $1::date - 30`, today).Scan(&o.People.Today, &o.People.Week, &o.People.Month)
	if err != nil {
		return o, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE days >= 3) FROM (
			SELECT m.discord_id, count(DISTINCT e.day) AS days
			FROM mood_entries e JOIN members m ON m.id = e.member_id
			WHERE e.day > $1::date - 7 GROUP BY m.discord_id) p`, today).Scan(&o.People.Repeat)
	if err != nil {
		return o, err
	}

	err = r.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM mood_entries WHERE day > $1::date - 7),
		       (SELECT count(*) FROM mood_entries WHERE day > $1::date - 7 AND message <> ''),
		       (SELECT count(*) FROM kudos WHERE day > $1::date - 7),
		       (SELECT count(*) FROM guilds WHERE active AND games_enabled),
		       (SELECT count(*) FROM game_proposals WHERE created_at >= $2::timestamptz - interval '7 days'),
		       (SELECT count(*) FROM game_answers WHERE created_at >= $2::timestamptz - interval '7 days')`, today, now).
		Scan(&o.Usage.Answers7d, &o.Usage.WithMessage7d, &o.Usage.Kudos7d, &o.Usage.GamesOn, &o.Usage.Proposals7d, &o.Usage.GameAnswers7d)
	if err != nil {
		return o, err
	}

	o.Failed7d = map[domain.DispatchKind]int{}
	rows, err := r.db.Query(ctx, `
		SELECT kind, coalesce(sum(failed), 0) FROM dispatches
		WHERE period > $1::date - 7 GROUP BY kind`, today)
	if err != nil {
		return o, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return o, err
		}
		o.Failed7d[domain.DispatchKind(k)] = n
	}
	if err := rows.Err(); err != nil {
		return o, err
	}

	if o.Days, err = r.days(ctx, today, 30); err != nil {
		return o, err
	}
	o.Weeks, err = r.weeks(ctx, today, now, 12, "")
	return o, err
}

func (r *AdminRepo) days(ctx context.Context, today time.Time, n int) ([]domain.AdminDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT d::date,
		       coalesce(a.answers, 0), coalesce(a.with_message, 0),
		       coalesce(s.recipients, 0), coalesce(s.failed, 0), coalesce(k.answered, 0)
		FROM generate_series($1::date - ($2::int - 1), $1::date, interval '1 day') d
		LEFT JOIN (SELECT day, count(*) AS answers, count(*) FILTER (WHERE message <> '') AS with_message
		           FROM mood_entries WHERE day > $1::date - $2::int GROUP BY day) a ON a.day = d::date
		LEFT JOIN (SELECT period, sum(recipients) AS recipients, sum(failed) AS failed
		           FROM dispatches WHERE kind = 'survey' AND period > $1::date - $2::int GROUP BY period) s ON s.period = d::date
		LEFT JOIN (SELECT e.day, count(*) AS answered FROM mood_entries e `+answeredSQL+`
		           WHERE e.day > $1::date - $2::int GROUP BY e.day) k ON k.day = d::date
		ORDER BY 1`, today, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AdminDay, error) {
		var d domain.AdminDay
		err := row.Scan(&d.Day, &d.Answers, &d.WithMessage, &d.Recipients, &d.Failed, &d.Answered)
		return d, err
	})
}

// weeks returns the last n ISO weeks up to today's, oldest first. With guildID it is that guild's
// surveys and answers only; without it, every guild's, plus joins, leaves and guilds with the bot.
func (r *AdminRepo) weeks(ctx context.Context, today, now time.Time, n int, guildID string) ([]domain.AdminWeek, error) {
	rows, err := r.db.Query(ctx, `
		WITH g AS (SELECT id FROM guilds WHERE $3 = '' OR discord_id = $3),
		w AS (SELECT wk::date AS week FROM generate_series(date_trunc('week', $1::date) - ($2::int - 1) * interval '1 week',
		                                                 date_trunc('week', $1::date), interval '1 week') wk)
		SELECT w.week,
		       coalesce((SELECT sum(d.recipients) FROM dispatches d JOIN g ON g.id = d.guild_id
		                 WHERE d.kind = 'survey' AND d.period >= w.week AND d.period < w.week + 7), 0),
		       (SELECT count(*) FROM mood_entries e JOIN g ON g.id = e.guild_id `+answeredSQL+`
		        WHERE e.day >= w.week AND e.day < w.week + 7),
		       (SELECT count(*) FROM mood_entries e JOIN g ON g.id = e.guild_id WHERE e.day >= w.week AND e.day < w.week + 7),
		       CASE WHEN $3 = '' THEN (SELECT count(*) FROM guilds WHERE created_at >= w.week AND created_at < w.week + 7) ELSE 0 END,
		       CASE WHEN $3 = '' THEN (SELECT count(*) FROM guilds WHERE left_at >= w.week AND left_at < w.week + 7)
		                            + (SELECT count(*) FROM guild_deletions WHERE reason = 'dashboard' AND deleted_at >= w.week AND deleted_at < w.week + 7)
		            ELSE 0 END,
		       CASE WHEN $3 = '' THEN (SELECT count(*) FROM guilds WHERE created_at < least(w.week + 7, $4::timestamptz)
		                                AND (left_at IS NULL OR left_at >= least(w.week + 7, $4::timestamptz))) ELSE 0 END
		FROM w ORDER BY w.week`, today, n, guildID, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AdminWeek, error) {
		var w domain.AdminWeek
		err := row.Scan(&w.Week, &w.Recipients, &w.Answered, &w.Answers, &w.Joins, &w.Leaves, &w.Guilds)
		return w, err
	})
}

const adminGuildSQL = `
	SELECT g.discord_id, g.name, g.active, g.created_at, g.left_at, g.language, g.timezone,
	       (SELECT count(*) FROM members m WHERE m.guild_id = g.id AND m.in_server),
	       (SELECT count(*) FROM members m WHERE m.guild_id = g.id AND m.in_server AND NOT m.active),
	       (SELECT count(DISTINCT e.member_id) FROM mood_entries e WHERE e.guild_id = g.id AND e.day > $1::date - 7),
	       (SELECT max(e.day) FROM mood_entries e WHERE e.guild_id = g.id),
	       coalesce((SELECT sum(d.recipients) FROM dispatches d WHERE d.guild_id = g.id AND d.kind = 'survey' AND d.period > $1::date - 7), 0),
	       (SELECT count(*) FROM mood_entries e ` + answeredSQL + ` WHERE e.guild_id = g.id AND e.day > $1::date - 7),
	       coalesce((SELECT sum(d.failed) FROM dispatches d WHERE d.guild_id = g.id AND d.period > $1::date - 7), 0)
	FROM guilds g`

func scanAdminGuild(row pgx.Row, now time.Time) (domain.AdminGuild, error) {
	var g domain.AdminGuild
	var active bool
	err := row.Scan(&g.DiscordID, &g.Name, &active, &g.CreatedAt, &g.LeftAt, &g.Language, &g.Timezone,
		&g.Members, &g.OptedOut, &g.Active7d, &g.LastAnswer, &g.Recipients, &g.Answered, &g.Failed7d)
	if err != nil {
		return g, err
	}
	g.Status = domain.GuildStatus(active, g.CreatedAt, g.LastAnswer, now)
	if g.LeftAt != nil {
		purge := g.LeftAt.Add(30 * 24 * time.Hour)
		g.PurgeAt = &purge
	}
	return g, nil
}

func (r *AdminRepo) Guilds(ctx context.Context, now time.Time) ([]domain.AdminGuild, error) {
	rows, err := r.db.Query(ctx, adminGuildSQL+` ORDER BY g.name`, domain.TruncateDay(now))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AdminGuild, error) { return scanAdminGuild(row, now) })
}

func (r *AdminRepo) Guild(ctx context.Context, guildID string, now time.Time) (domain.AdminGuildDetail, error) {
	today := domain.TruncateDay(now)
	var d domain.AdminGuildDetail
	g, err := scanAdminGuild(r.db.QueryRow(ctx, adminGuildSQL+` WHERE g.discord_id = $2`, today, guildID), now)
	if err != nil {
		return d, notFound(err)
	}
	d.AdminGuild = g
	s := &d.Settings
	var days int16
	err = r.db.QueryRow(ctx, `
		SELECT anonymous, survey_hour, working_days, report_frequency, report_dm, survey_message <> '',
		       survey_roles_mode, game_roles_mode, games_enabled, kudos_channel_id <> '',
		       (SELECT count(*) FROM kudos k WHERE k.guild_id = g.id AND k.day > $1::date - 7),
		       (SELECT count(*) FROM mood_entries e WHERE e.guild_id = g.id AND e.day > $1::date - 30),
		       (SELECT count(*) FROM mood_entries e WHERE e.guild_id = g.id AND e.day > $1::date - 30 AND e.message <> ''),
		       (SELECT max(period) FROM dispatches d WHERE d.guild_id = g.id AND d.kind = 'report'),
		       (SELECT count(*) FROM game_answers a JOIN game_rounds r ON r.id = a.round_id
		        WHERE r.guild_id = g.id AND a.created_at >= $3::timestamptz - interval '7 days')
		FROM guilds g WHERE g.discord_id = $2`, today, guildID, now).Scan(
		&s.Anonymous, &s.SurveyHour, &days, &s.ReportFrequency, &s.ReportDM, &s.CustomMessage,
		&s.SurveyRoles, &s.GameRoles, &s.GamesEnabled, &s.KudosChannel,
		&d.Kudos7d, &d.Answers30d, &d.WithMessage, &d.LastReport, &d.GameAnswers7)
	if err != nil {
		return d, notFound(err)
	}
	s.WorkingDays = domain.Weekdays(days)
	d.Weeks, err = r.weeks(ctx, today, now, 8, guildID)
	return d, err
}

// Deliveries counts, per day, the people each kind of send went to and the DMs that failed.
func (r *AdminRepo) Deliveries(ctx context.Context, now time.Time, n int) ([]domain.AdminDeliveryDay, error) {
	today := domain.TruncateDay(now)
	rows, err := r.db.Query(ctx, `
		SELECT d::date, coalesce(x.kind, ''), coalesce(x.sent, 0), coalesce(x.failed, 0)
		FROM generate_series($1::date - ($2::int - 1), $1::date, interval '1 day') d
		LEFT JOIN (SELECT period, kind, sum(recipients) AS sent, sum(failed) AS failed FROM dispatches
		           WHERE period > $1::date - $2::int AND recipients IS NOT NULL GROUP BY period, kind) x ON x.period = d::date
		ORDER BY 1, 2`, today, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AdminDeliveryDay
	for rows.Next() {
		var day time.Time
		var kind string
		var sent, failed int
		if err := rows.Scan(&day, &kind, &sent, &failed); err != nil {
			return nil, err
		}
		if len(out) == 0 || !out[len(out)-1].Day.Equal(day) {
			out = append(out, domain.AdminDeliveryDay{Day: day, Sent: map[domain.DispatchKind]int{}, Failed: map[domain.DispatchKind]int{}})
		}
		if kind != "" {
			out[len(out)-1].Sent[domain.DispatchKind(kind)] = sent
			out[len(out)-1].Failed[domain.DispatchKind(kind)] = failed
		}
	}
	return out, rows.Err()
}

func (r *AdminRepo) Database(ctx context.Context) (domain.AdminDatabase, error) {
	var d domain.AdminDatabase
	var dirty bool
	err := r.db.QueryRow(ctx, `
		SELECT current_setting('server_version'),
		       pg_database_size(current_database()),
		       (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()),
		       current_setting('max_connections')::int,
		       (SELECT version FROM schema_migrations LIMIT 1),
		       (SELECT dirty FROM schema_migrations LIMIT 1)`).
		Scan(&d.Version, &d.SizeBytes, &d.Connections, &d.MaxConnections, &d.Migration, &dirty)
	if err != nil {
		return d, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT relname, pg_total_relation_size(relid) FROM pg_stat_user_tables
		ORDER BY 2 DESC LIMIT 1`).Scan(&d.LargestTable, &d.LargestBytes)
	if err == pgx.ErrNoRows {
		err = nil
	}
	return d, err
}

func (r *AdminRepo) Settings(ctx context.Context) (domain.AdminSettings, error) {
	var s domain.AdminSettings
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM guilds WHERE active`).Scan(&s.Guilds); err != nil {
		return s, err
	}
	for _, q := range []struct {
		dst  *[]domain.Count
		expr string
	}{
		{&s.Language, `language`},
		{&s.Timezone, `timezone`},
		{&s.SurveyHour, `lpad(survey_hour::text, 2, '0') || ':00'`},
		{&s.Anonymous, `CASE WHEN anonymous THEN 'anonymous' ELSE 'public' END`},
		{&s.ReportFrequency, `report_frequency`},
		{&s.ReportDM, `CASE WHEN report_dm THEN 'on' ELSE 'off' END`},
		{&s.SurveyRoles, `survey_roles_mode`},
	} {
		rows, err := r.db.Query(ctx, `SELECT `+q.expr+` AS v, count(*) FROM guilds WHERE active GROUP BY v ORDER BY 2 DESC, 1`)
		if err != nil {
			return s, err
		}
		*q.dst, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Count, error) {
			var c domain.Count
			err := row.Scan(&c.Value, &c.N)
			return c, err
		})
		if err != nil {
			return s, err
		}
	}
	var games, kudos, message, days int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE games_enabled), count(*) FILTER (WHERE kudos_channel_id <> ''),
		       count(*) FILTER (WHERE survey_message <> ''), count(*) FILTER (WHERE working_days <> 62)
		FROM guilds WHERE active`).Scan(&games, &kudos, &message, &days)
	s.Extras = []domain.Count{{Value: "games", N: games}, {Value: "kudos_channel", N: kudos}, {Value: "custom_message", N: message}, {Value: "custom_days", N: days}}
	return s, err
}
