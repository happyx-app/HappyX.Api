package domain

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// «Who's who?» is a weekly game. On Monday every member gets an icebreaker question; they
// answer until Wednesday. From Thursday to Sunday the guild's members guess on the web who wrote
// each answer, and once the week is over the authors and the ranking are revealed. Answers are
// never anonymous: everyone is told their name comes out at the end of the week.

const (
	MaxGameQuestionLength = 300
	MaxGameAnswerLength   = 200
	MaxGameProposalLength = 200
	// MinGameAnswers is how many answers a round needs for guessing to make sense.
	MinGameAnswers = 3
	// gameAnswerDays is how long members have to answer, from Monday.
	gameAnswerDays = 3
)

type GamePhase string

const (
	GameAnswering GamePhase = "answering"
	GameGuessing  GamePhase = "guessing"
	GameFinished  GamePhase = "finished"
)

// GameRound is one guild's question for one week.
type GameRound struct {
	GuildDiscordID string
	Week           time.Time // the Monday the round starts, date only
	Question       string
	// Proposer is who proposed the question, when it was drawn from the members' proposals
	// rather than the bot's list. Empty if they left the guild since.
	ProposerDiscordID string
	ProposerUsername  string
	Proposed          bool
	CreatedAt         time.Time
}

func (r GameRound) Validate() error {
	if err := validateSnowflake("discord_guild_id", r.GuildDiscordID); err != nil {
		return err
	}
	if r.Week.IsZero() || r.Week.Weekday() != time.Monday {
		return invalid("week must be a Monday")
	}
	if strings.TrimSpace(r.Question) == "" || utf8.RuneCountInString(r.Question) > MaxGameQuestionLength {
		return invalid("question must be 1-%d characters", MaxGameQuestionLength)
	}
	return nil
}

// GuessingStarts is the first day of guessing (Thursday).
func (r GameRound) GuessingStarts() time.Time { return r.Week.AddDate(0, 0, gameAnswerDays) }

// LastDay is the round's Sunday; the results are out the day after.
func (r GameRound) LastDay() time.Time { return r.Week.AddDate(0, 0, 6) }

// Phase is where the round is on today, a date in the guild's timezone.
func (r GameRound) Phase(today time.Time) GamePhase {
	switch {
	case today.Before(r.GuessingStarts()):
		return GameAnswering
	case !today.After(r.LastDay()):
		return GameGuessing
	default:
		return GameFinished
	}
}

// ProposalsOpen reports whether members can propose next week's question on today: while this
// week's round is being guessed, Thursday to Sunday.
func ProposalsOpen(today time.Time) bool {
	return !TruncateDay(today).Before(WeekStart(today).AddDate(0, 0, gameAnswerDays))
}

// WeekStart is the Monday of day's week.
func WeekStart(day time.Time) time.Time {
	day = TruncateDay(day)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// GameAnswer is one member's answer to a round. ID is opaque and random, so listing answers by
// ID gives nothing away.
type GameAnswer struct {
	ID              string
	GuildDiscordID  string
	Week            time.Time
	MemberDiscordID string
	Username        string
	Text            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (a GameAnswer) Validate() error {
	if err := validateSnowflake("discord_user_id", a.MemberDiscordID); err != nil {
		return err
	}
	if strings.TrimSpace(a.Text) == "" || utf8.RuneCountInString(a.Text) > MaxGameAnswerLength {
		return invalid("answer must be 1-%d characters", MaxGameAnswerLength)
	}
	return nil
}

// GameProposal is a question a member proposes for a later week's round. On that Monday one of
// the week's proposals is drawn at random; the others are dropped.
type GameProposal struct {
	GuildDiscordID  string
	Week            time.Time // the Monday of the round it is for
	MemberDiscordID string
	Username        string
	Text            string
}

func (p GameProposal) Validate() error {
	if err := validateSnowflake("discord_user_id", p.MemberDiscordID); err != nil {
		return err
	}
	if strings.TrimSpace(p.Text) == "" || utf8.RuneCountInString(p.Text) > MaxGameProposalLength {
		return invalid("question must be 1-%d characters", MaxGameProposalLength)
	}
	return nil
}

// GameGuess is a member's bet on who wrote an answer.
type GameGuess struct {
	GuesserDiscordID string
	AnswerID         string
	GuessedDiscordID string
}

// GameScore is one guesser's result in a finished round.
type GameScore struct {
	DiscordID string
	Username  string
	Correct   int
	Guesses   int
}

// Ranking scores everyone who guessed, best first: most right, then fewest guesses, then name.
// Guesses on answers that are no longer in the round (their author paused HappyX) don't count.
func Ranking(answers []GameAnswer, guesses []GameGuess, usernames map[string]string) []GameScore {
	author := map[string]string{}
	for _, a := range answers {
		author[a.ID] = a.MemberDiscordID
	}
	scores := map[string]*GameScore{}
	for _, g := range guesses {
		who, ok := author[g.AnswerID]
		if !ok {
			continue
		}
		s := scores[g.GuesserDiscordID]
		if s == nil {
			s = &GameScore{DiscordID: g.GuesserDiscordID, Username: usernames[g.GuesserDiscordID]}
			scores[g.GuesserDiscordID] = s
		}
		s.Guesses++
		if who == g.GuessedDiscordID {
			s.Correct++
		}
	}
	out := make([]GameScore, 0, len(scores))
	for _, s := range scores {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Correct != b.Correct {
			return a.Correct > b.Correct
		}
		if a.Guesses != b.Guesses {
			return a.Guesses < b.Guesses
		}
		if x, y := strings.ToLower(a.Username), strings.ToLower(b.Username); x != y {
			return x < y
		}
		return a.DiscordID < b.DiscordID
	})
	return out
}
