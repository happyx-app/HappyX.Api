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
}

// GuildExport is a guild's data as its managers may download it.
type GuildExport struct {
	Guild   Guild
	Members []Member
	Moods   []ExportedMood
	Kudos   []Kudos
	Rounds  []ExportedRound
}

// ExportedMood is a survey answer in a guild export. Username is empty when the answer is
// anonymous.
type ExportedMood struct {
	Day      time.Time
	Mood     Mood
	Message  string
	Username string
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
//     given while it wasn't either, and the author still takes part (like reports);
//   - «Who's who?» answers appear once guessing starts, and their authors (and who proposed the
//     question) once the round is finished, like on the game page.
//
// today is the current date in the guild's timezone.
func BuildGuildExport(d GuildData, today time.Time) GuildExport {
	out := GuildExport{Guild: d.Guild, Members: d.Members, Kudos: d.Kudos, Moods: []ExportedMood{}, Rounds: []ExportedRound{}}

	named := map[string]string{}
	if !d.Guild.Settings.Anonymous {
		for _, m := range d.Members {
			if m.Active {
				named[m.DiscordID] = m.Username
			}
		}
	}
	for _, e := range d.Moods {
		em := ExportedMood{Day: e.Day, Mood: e.Mood, Message: e.Message}
		if !e.Anonymous {
			em.Username = named[e.MemberDiscordID]
		}
		out.Moods = append(out.Moods, em)
	}
	// Within a day, anonymous answers are sorted by what they say, so their order can't be
	// matched with the order members joined or answered in.
	sort.SliceStable(out.Moods, func(i, j int) bool {
		a, b := out.Moods[i], out.Moods[j]
		if !a.Day.Equal(b.Day) {
			return a.Day.Before(b.Day)
		}
		if a.Username != b.Username {
			return a.Username < b.Username
		}
		if a.Mood != b.Mood {
			return a.Mood < b.Mood
		}
		return a.Message < b.Message
	})

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
