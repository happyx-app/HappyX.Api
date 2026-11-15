package domain

import (
	"sort"
	"time"
)

// Personal data (GDPR): anyone can download everything HappyX keeps about them and erase it, and
// the people who manage a server can download the server's data. A server's export keeps the
// same anonymity as the dashboard, so it never tells admins more than they could already see.

// UserData is everything HappyX keeps about one person, in every guild.
type UserData struct {
	DiscordID string
	// ReportsFor are the guilds whose reports the person receives as the registered admin.
	ReportsFor []Guild
	Guilds     []MemberData
	// Vacations are the person's own and the ones admins set for them, past ones included.
	Vacations []Vacation
}

// MemberData is everything HappyX keeps about one person in one guild.
type MemberData struct {
	Guild         Guild
	Member        Member
	Moods         []MoodEntry
	KudosGiven    []Kudos
	KudosReceived []Kudos
	GameAnswers   []RoundAnswer
	GameGuesses   []MadeGuess
	GameProposals []GameProposal
	CanvasStrokes []CanvasStroke
	// TeamNotes are what they told the team after a report, as an admin.
	TeamNotes []TeamNote
}

// RoundAnswer is an answer to a «Who's who?» round, with the round's question.
type RoundAnswer struct {
	Week            time.Time
	Question        string
	Text            string
	MemberDiscordID string
	Username        string
}

// MadeGuess is who someone bet wrote an answer.
type MadeGuess struct {
	Week            time.Time
	Question        string
	Answer          string
	GuessedUsername string
}

// GuildData is everything HappyX keeps about a guild, as stored. BuildGuildExport decides what of
// it leaves HappyX.
type GuildData struct {
	Guild Guild
	// Members are everyone the bot has seen in the guild, taking part or not.
	Members []Member
	Moods   []MoodEntry
	Kudos   []Kudos
	Rounds  []GameRound
	// Answers are the rounds' answers by members still taking part, in the game's random order.
	Answers []RoundAnswer
	// CanvasStrokes are the team canvas' strokes, oldest first.
	CanvasStrokes []CanvasStroke
	// TeamNotes are what the admins told the team after reports, oldest first. They were
	// published in a channel, so they leave with their authors.
	TeamNotes []TeamNote
}

// GuildExport is a guild's data as its managers may download it.
type GuildExport struct {
	Guild   Guild
	Members []Member
	// Moods are the answers that show their author, one by one.
	Moods []ExportedMood
	// AnonymousDays sum up the anonymous answers by day, as the dashboard shows them: how many,
	// and their average unless the day is withheld. AnonymousComments are their comments with
	// neither day nor mood, the withheld days' left out (see BuildReport).
	AnonymousDays     []ExportedDay
	AnonymousComments []string
	Kudos             []Kudos
	Rounds            []ExportedRound
	// CanvasStrokes are the team canvas' strokes with their authors, as moderators see them.
	CanvasStrokes []CanvasStroke
	TeamNotes     []TeamNote
}

// ExportedMood is a survey answer that names its author in a guild export.
type ExportedMood struct {
	Day      time.Time
	Mood     Mood
	Message  string
	Username string
}

// ExportedDay is a day's anonymous answers in a guild export. Average is 0 when Withheld.
type ExportedDay struct {
	Day      time.Time
	Answers  int
	Average  float64
	Withheld bool
}

// ExportedRound is a «Who's who?» round in a guild export, with what the game has revealed so far.
type ExportedRound struct {
	Week     time.Time
	Question string
	// ProposedBy names the member who proposed the question, once the round is finished.
	ProposedBy string
	Answers    []ExportedAnswer
}

// ExportedAnswer is an answer in a guild export. Username is empty until the round is finished.
type ExportedAnswer struct {
	Text     string
	Username string
}

// BuildGuildExport applies the dashboard's rules to the guild's data:
//   - a survey answer names its author only if the guild is not anonymous now, the answer was
//     given while it wasn't either, and the author still takes part (like reports). The rest
//     are summed up by day, and their comments go apart without day or mood, with the same
//     threshold as the dashboard;
//   - «Who's who?» answers appear once guessing starts, and their authors (and who proposed the
//     question) once the round is finished, like on the game page.
//
// today is the current date in the guild's timezone.
func BuildGuildExport(d GuildData, today time.Time) GuildExport {
	out := GuildExport{Guild: d.Guild, Members: d.Members, Kudos: d.Kudos, Moods: []ExportedMood{}, AnonymousDays: []ExportedDay{}, AnonymousComments: []string{}, Rounds: []ExportedRound{}, CanvasStrokes: d.CanvasStrokes, TeamNotes: d.TeamNotes}

	named := map[string]string{}
	if !d.Guild.Settings.Anonymous {
		for _, m := range d.Members {
			if m.Active {
				named[m.DiscordID] = m.Username
			}
		}
	}
	type day struct {
		answers, sum int
		people       map[string]struct{}
		k            int // the highest threshold among the day's answers
		comments     []string
	}
	days := map[time.Time]*day{}
	for _, e := range d.Moods {
		if name := named[e.MemberDiscordID]; name != "" && !e.Anonymous {
			out.Moods = append(out.Moods, ExportedMood{Day: e.Day, Mood: e.Mood, Message: e.Message, Username: name})
			continue
		}
		dd := days[TruncateDay(e.Day)]
		if dd == nil {
			dd = &day{people: map[string]struct{}{}}
			days[TruncateDay(e.Day)] = dd
		}
		dd.answers++
		dd.sum += int(e.Mood)
		dd.people[e.MemberDiscordID] = struct{}{}
		dd.k = max(dd.k, e.AnonymityThreshold)
		if e.Message != "" {
			dd.comments = append(dd.comments, e.Message)
		}
	}
	sort.SliceStable(out.Moods, func(i, j int) bool {
		a, b := out.Moods[i], out.Moods[j]
		if !a.Day.Equal(b.Day) {
			return a.Day.Before(b.Day)
		}
		return a.Username < b.Username
	})
	for dt, dd := range days {
		x := ExportedDay{Day: dt, Answers: dd.answers, Withheld: WithheldDay(len(dd.people), max(dd.k, d.Guild.Settings.AnonymityThreshold))}
		if !x.Withheld {
			x.Average = float64(dd.sum) / float64(dd.answers)
			out.AnonymousComments = append(out.AnonymousComments, dd.comments...)
		}
		out.AnonymousDays = append(out.AnonymousDays, x)
	}
	sort.Slice(out.AnonymousDays, func(i, j int) bool { return out.AnonymousDays[i].Day.Before(out.AnonymousDays[j].Day) })
	// Sorted by what they say, so their order tells nothing about when they were written.
	sort.Strings(out.AnonymousComments)

	byWeek := map[time.Time][]RoundAnswer{}
	for _, a := range d.Answers {
		byWeek[a.Week] = append(byWeek[a.Week], a)
	}
	for _, r := range d.Rounds {
		phase := r.Phase(today)
		er := ExportedRound{Week: r.Week, Question: r.Question, Answers: []ExportedAnswer{}}
		if phase == GameFinished && r.Proposed {
			er.ProposedBy = r.ProposerUsername
		}
		if phase != GameAnswering {
			for _, a := range byWeek[r.Week] {
				ea := ExportedAnswer{Text: a.Text}
				if phase == GameFinished {
					ea.Username = a.Username
				}
				er.Answers = append(er.Answers, ea)
			}
		}
		out.Rounds = append(out.Rounds, er)
	}
	return out
}
