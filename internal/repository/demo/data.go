// Package demo is a made-up server, «Equipo Demo», kept in memory for the public demo of the
// dashboard. It implements the usecase repositories, so the demo runs through the same code as
// real servers, but it never touches the database: the bot, the admin dashboard, exports and
// backups never see it, and nothing the demo serves can come from a real server.
//
// The history is generated from the date: every day's answers come from a hash of the day and
// the person, so they stay the same as time goes by, and the last six months always end today.
package demo

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// GuildID is the demo server's reserved ID. It only exists in this package.
const GuildID = "100000000000000001"

// HistoryDays is how far back the demo's answers go.
const HistoryDays = 183

const timezone = "Europe/Madrid"

type person struct {
	id, name string
	base     float64 // their usual mood
	roles    []string
	optedOut bool
}

func userID(n int) string { return fmt.Sprintf("1000000000000001%02d", n) }
func roleID(n int) string { return fmt.Sprintf("1000000000000002%02d", n) }
func chanID(n int) string { return fmt.Sprintf("1000000000000003%02d", n) }

var (
	roleProduct = roleID(1)
	roleDev     = roleID(2)
	roleDesign  = roleID(3)
	roleExt     = roleID(4)
)

// The demo is seen as Laura, the server's main admin.
var people = []person{
	{userID(1), "laura", 4.0, []string{roleProduct}, false},
	{userID(2), "marc", 3.8, []string{roleDev}, false},
	{userID(3), "nuria", 4.2, []string{roleDesign}, false},
	{userID(4), "javi", 3.5, []string{roleDev}, false},
	{userID(5), "sofia", 4.1, []string{roleProduct}, false},
	{userID(6), "pau", 3.6, []string{roleDev}, false},
	{userID(7), "elena", 4.3, []string{roleDesign}, false},
	{userID(8), "dani", 3.4, []string{roleDev}, false},
	{userID(9), "irene", 3.9, []string{roleProduct}, false},
	{userID(10), "carlos", 3.7, []string{roleDev}, false},
	{userID(11), "marta", 4.0, []string{roleExt}, false},
	{userID(12), "alex", 3.6, []string{roleDev}, true},
}

// Viewer is who the demo is seen as: Laura, the main admin.
func Viewer() domain.Viewer {
	return domain.Viewer{DiscordID: people[0].id, Username: "laura", ManagedGuilds: []string{GuildID}}
}

var roles = []domain.Role{
	{DiscordID: roleProduct, Name: "Producto", Color: 0x7C5CFF, Position: 4},
	{DiscordID: roleDev, Name: "Desarrollo", Color: 0x2D9CDB, Position: 3},
	{DiscordID: roleDesign, Name: "Diseño", Color: 0xF2B705, Position: 2},
	{DiscordID: roleExt, Name: "Externos", Color: 0, Position: 1},
}

var channels = []domain.Channel{
	{DiscordID: chanID(1), Name: "general", Category: "Equipo", Position: 0, CanPost: true},
	{DiscordID: chanID(2), Name: "kudos", Category: "Equipo", Position: 1, CanPost: true},
	{DiscordID: chanID(3), Name: "random", Category: "Equipo", Position: 2, CanPost: true},
	{DiscordID: chanID(4), Name: "anuncios", Category: "Info", Position: 3, CanPost: false},
}

var grants = []domain.AccessGrant{
	{Kind: domain.GrantRole, TargetID: roleProduct, Level: domain.AccessView},
	{Kind: domain.GrantUser, TargetID: userID(2), Level: domain.AccessManage},
}

func guild(lang string, created time.Time) domain.Guild {
	s := domain.DefaultGuildSettings()
	s.Language = lang
	s.Timezone = timezone
	s.KudosChannelID = chanID(2)
	s.GamesEnabled = true
	s.SurveyRoles = domain.RoleFilter{Mode: domain.RolesExcept, RoleIDs: []string{roleExt}}
	s.GameRoles = domain.RoleFilter{Mode: domain.RolesEveryone}
	return domain.Guild{
		DiscordID: GuildID, Name: texts[lang].guildName, AdminDiscordID: people[0].id, Settings: s, Active: true,
		CreatedAt: created, UpdatedAt: created,
	}
}

// data is one day's snapshot of the demo server.
type data struct {
	text       textSet
	today      time.Time // in the server's timezone
	surveyed   bool      // whether today's survey went out yet
	guild      domain.Guild
	members    []domain.Member
	moods      []domain.MoodEntry // oldest first
	kudos      []domain.Kudos     // oldest first
	dispatches []domain.DispatchStat
	rounds     map[time.Time]domain.GameRound
	answers    map[time.Time][]domain.GameAnswer
	guesses    map[time.Time][]domain.GameGuess
	proposals  map[time.Time][]domain.GameProposal
}

// Store keeps the demo's data in one language, generated again when the day changes.
type Store struct {
	lang string
	now  func() time.Time
	mu   sync.Mutex
	data *data
}

// NewStore makes the demo in lang, one of Languages; an unknown one gets the first.
func NewStore(lang string, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	if _, ok := texts[lang]; !ok {
		lang = Languages[0]
	}
	return &Store{lang: lang, now: now}
}

func (s *Store) current() *data {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := s.now().In(loc)
	today := domain.TruncateDay(now)
	// The survey goes out at 13:00, so today's answers only show from then.
	surveyed := now.Hour() >= domain.DefaultGuildSettings().SurveyHour
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil || !s.data.today.Equal(today) || s.data.surveyed != surveyed {
		s.data = generate(s.lang, today, surveyed)
	}
	return s.data
}

// rnd is a stable number in [0, 1) for the given parts.
func rnd(parts ...any) float64 {
	h := fnv.New64a()
	for _, p := range parts {
		fmt.Fprintf(h, "%v|", p)
	}
	// FNV's high bits barely move when only the last characters change; mix them (splitmix64).
	x := h.Sum64()
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return float64(x>>11) / float64(1<<53)
}

func pick[T any](xs []T, parts ...any) T { return xs[int(rnd(parts...)*float64(len(xs)))] }

// teamMood is how the whole team feels on day, beyond each person's usual mood: slow waves of
// good and harder stretches, plus Mondays and Fridays. It depends only on the date, so the
// history never changes.
func teamMood(day time.Time) float64 {
	n := float64(day.Unix() / 86400)
	waves := 0.45*math.Sin(2*math.Pi*n/97) + 0.25*math.Sin(2*math.Pi*n/41+1)
	weekday := map[time.Weekday]float64{time.Monday: -0.3, time.Tuesday: -0.05, time.Thursday: 0.05, time.Friday: 0.35}[day.Weekday()]
	return waves + weekday
}

func generate(lang string, today time.Time, surveyedToday bool) *data {
	start := today.AddDate(0, 0, -HistoryDays)
	d := &data{
		text: texts[lang], today: today, surveyed: surveyedToday, guild: guild(lang, start.Add(9*time.Hour)),
		rounds: map[time.Time]domain.GameRound{}, answers: map[time.Time][]domain.GameAnswer{},
		guesses: map[time.Time][]domain.GameGuess{}, proposals: map[time.Time][]domain.GameProposal{},
	}
	for _, p := range people {
		d.members = append(d.members, domain.Member{
			GuildDiscordID: GuildID, DiscordID: p.id, Username: p.name, Active: !p.optedOut,
			RoleIDs: p.roles, InServer: true, CreatedAt: d.guild.CreatedAt,
		})
	}
	for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
		if !d.guild.Settings.IsWorkingDay(day) || (day.Equal(today) && !surveyedToday) {
			continue
		}
		d.addDay(day)
	}
	d.addGame()
	return d
}

func (d *data) addDay(day time.Time) {
	key := day.Format("2006-01-02")
	recipients := 0
	for _, p := range people {
		// Alex said «don't ask me again»; externals don't get the survey.
		if p.optedOut {
			continue
		}
		if len(p.roles) > 0 && p.roles[0] == roleExt {
			continue
		}
		recipients++
		answerRate := 0.84
		if teamMood(day) < -0.4 {
			answerRate = 0.72 // fewer answer when things are hard
		}
		if rnd("answered", key, p.id) >= answerRate || (day.Equal(d.today) && rnd("late", key, p.id) < 0.3) {
			continue // the rest of today's answers come later
		}
		v := p.base + teamMood(day) + (rnd("mood", key, p.id)*2-1)*0.95
		if rnd("bad-day", key, p.id) < 0.04 {
			v -= 1.8 // everyone has a bad day now and then
		}
		mood := domain.Mood(max(1, min(5, int(math.Round(v)))))
		e := domain.MoodEntry{
			GuildDiscordID: GuildID, MemberDiscordID: p.id, Day: day, Mood: mood, Anonymous: true,
			CreatedAt: day.Add(time.Duration(13*60+int(rnd("at", key, p.id)*240)) * time.Minute),
		}
		if rnd("comment", key, p.id) < 0.2 {
			e.Message = pick(d.text.comments[mood], "text", key, p.id)
		}
		e.UpdatedAt = e.CreatedAt
		d.moods = append(d.moods, e)
	}
	failed := 0
	if rnd("failed", key) < 0.06 {
		failed = 1
	}
	d.dispatches = append(d.dispatches, domain.DispatchStat{Day: day, Recipients: recipients, Failed: failed})

	// Kudos: none on some days, a couple on others.
	for i := 0; i < 2; i++ {
		if rnd("kudos", key, i) >= 0.3 {
			continue
		}
		from := pick(people, "giver", key, i)
		to := pick(people, "receiver", key, i)
		if from.id == to.id {
			continue
		}
		d.kudos = append(d.kudos, domain.Kudos{
			GuildDiscordID: GuildID, GiverDiscordID: from.id, GiverUsername: from.name,
			ReceiverDiscordID: to.id, ReceiverUsername: to.name, Day: day,
			Message:   pick(d.text.kudos, "kudos-text", key, i),
			CreatedAt: day.Add(time.Duration(10*60+int(rnd("kudos-at", key, i)*420)) * time.Minute),
		})
	}
}

// addGame fills «Who's who?» for this week and the three before, and next week's proposals.
func (d *data) addGame() {
	week := domain.WeekStart(d.today)
	for n := 0; n <= 3; n++ {
		w := week.AddDate(0, 0, -7*n)
		key := w.Format("2006-01-02")
		q := int(w.Unix()/86400/7) % len(d.text.questions)
		r := domain.GameRound{GuildDiscordID: GuildID, Week: w, Question: d.text.questions[q], CreatedAt: w.Add(9 * time.Hour)}
		if n == 1 {
			r.Proposed, r.ProposerDiscordID, r.ProposerUsername = true, people[6].id, people[6].name
		}
		d.rounds[w] = r
		answeringOver := n > 0 || r.Phase(d.today) != domain.GameAnswering
		var players []person
		for i, p := range people {
			if p.optedOut {
				continue
			}
			// This week, while answering is open, only some answered yet. Laura did.
			if !answeringOver && i > 0 && rnd("early", key, p.id) > 0.5 {
				continue
			}
			if answeringOver && i > 0 && rnd("plays", key, p.id) > 0.8 {
				continue
			}
			players = append(players, p)
		}
		for _, p := range players {
			texts := d.text.answers[q]
			d.answers[w] = append(d.answers[w], domain.GameAnswer{
				ID: fmt.Sprintf("%016x", uint64(rnd("answer-id", key, p.id)*(1<<53))), GuildDiscordID: GuildID, Week: w,
				MemberDiscordID: p.id, Username: p.name, Text: texts[(personIndex(p.id)+n)%len(texts)],
				CreatedAt: w.Add(time.Duration(10+personIndex(p.id)) * time.Hour),
			})
		}
		sort.Slice(d.answers[w], func(i, j int) bool { return d.answers[w][i].ID < d.answers[w][j].ID })
		// Everyone else guesses; Laura leaves this week's guesses to whoever tries the demo.
		if !answeringOver {
			continue
		}
		for _, g := range players {
			if n == 0 && g.id == people[0].id {
				continue
			}
			for _, a := range d.answers[w] {
				if a.MemberDiscordID == g.id || rnd("guesses", key, g.id, a.ID) > 0.85 {
					continue
				}
				guess := a.MemberDiscordID
				if rnd("right", key, g.id, a.ID) > 0.45 {
					other := pick(players, "wrong", key, g.id, a.ID)
					if other.id != g.id {
						guess = other.id
					}
				}
				d.guesses[w] = append(d.guesses[w], domain.GameGuess{GuesserDiscordID: g.id, AnswerID: a.ID, GuessedDiscordID: guess})
			}
		}
	}
	if domain.ProposalsOpen(d.today) {
		next := week.AddDate(0, 0, 7)
		for i, p := range []person{people[3], people[8], people[4]} {
			d.proposals[next] = append(d.proposals[next], domain.GameProposal{
				GuildDiscordID: GuildID, Week: next, MemberDiscordID: p.id, Username: p.name, Text: d.text.proposals[i],
			})
		}
	}
}

func personIndex(id string) int {
	for i, p := range people {
		if p.id == id {
			return i
		}
	}
	return 0
}
