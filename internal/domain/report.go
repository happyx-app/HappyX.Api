package domain

import (
	"slices"
	"strings"
	"time"
)

// Report summarises a guild's moods over a period. It is the data the bot turns into
// the image sent to the guild admin.
type Report struct {
	GuildDiscordID string
	From, To       time.Time // inclusive dates
	Anonymous      bool
	// AnonymityThreshold is the guild's current threshold, or the highest one a withheld day
	// was judged by when that is higher (after the guild lowered it). Withheld is set when
	// every answer falls on withheld days, and then the report keeps only how many answered.
	AnonymityThreshold int
	Withheld           bool
	Responses          int
	Respondents        int
	// Average, Distribution and Messages leave out the withheld days (see BuildReport), so
	// Average is 0 when Withheld, and Distribution may add up to fewer than Responses.
	Average      float64
	Distribution map[Mood]int // count per mood
	Days         []DayStat
	Messages     []ReportMessage
	Kudos        KudosSummary
	// Participation compares answers with surveys sent, on the days the bot reported how many
	// people it sent the survey to. Nil when it reported none in the period.
	Participation *Participation
	// Audience is how many people get the survey now (see GuildHealth.Audience), to say how many
	// of them answered. 0 when unknown.
	Audience int
}

// Participation is how many survey DMs went out and how many of them got an answer.
type Participation struct {
	Recipients int
	Answered   int
}

// AddParticipation fills each day's recipients from the survey dispatches and sums the
// days that have them. Answers on days without a known number of recipients are left out
// of both sides, so the rate is never above what the bot actually sent.
func (r *Report) AddParticipation(sent []DispatchStat) {
	byDay := map[time.Time]int{}
	for _, d := range sent {
		byDay[TruncateDay(d.Day)] = d.Recipients
	}
	for i, d := range r.Days {
		n, ok := byDay[TruncateDay(d.Day)]
		if !ok {
			continue
		}
		r.Days[i].Recipients = &n
		if r.Participation == nil {
			r.Participation = &Participation{}
		}
		r.Participation.Recipients += n
		r.Participation.Answered += min(d.Responses, n)
	}
}

// KudosSummary counts the kudos given in a report's period. Kudos are public, so they keep
// their names even when the guild's moods are anonymous.
type KudosSummary struct {
	Count     int
	Givers    int
	Receivers int
	// Highlights are the latest kudos, newest first, at most MaxKudosHighlights.
	Highlights []Kudos
	// TopReceivers are who got the most kudos, most first, at most MaxKudosTopReceivers.
	TopReceivers []KudosCount
}

// KudosCount is how many kudos one member received.
type KudosCount struct {
	DiscordID string
	Username  string
	Count     int
}

const (
	MaxKudosHighlights   = 5
	MaxKudosTopReceivers = 3
)

// SummarizeKudos counts kudos (ordered oldest first) and keeps the latest as highlights.
func SummarizeKudos(kudos []Kudos) KudosSummary {
	s := KudosSummary{Count: len(kudos), Highlights: []Kudos{}, TopReceivers: []KudosCount{}}
	givers := map[string]struct{}{}
	received := map[string]*KudosCount{}
	for _, k := range kudos {
		givers[k.GiverDiscordID] = struct{}{}
		c := received[k.ReceiverDiscordID]
		if c == nil {
			c = &KudosCount{DiscordID: k.ReceiverDiscordID}
			received[k.ReceiverDiscordID] = c
		}
		c.Count++
		c.Username = k.ReceiverUsername // the latest name wins
	}
	s.Givers, s.Receivers = len(givers), len(received)
	for _, c := range received {
		s.TopReceivers = append(s.TopReceivers, *c)
	}
	slices.SortFunc(s.TopReceivers, func(a, b KudosCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
	s.TopReceivers = s.TopReceivers[:min(len(s.TopReceivers), MaxKudosTopReceivers)]
	for i := len(kudos) - 1; i >= 0 && len(s.Highlights) < MaxKudosHighlights; i-- {
		s.Highlights = append(s.Highlights, kudos[i])
	}
	return s
}

type DayStat struct {
	Day       time.Time
	Responses int
	// Average is 0 when Withheld: that day's anonymous answers came from fewer people than
	// the guild's threshold.
	Average  float64
	Withheld bool
	// AnonymityThreshold is the one the day was judged by: the highest of its answers' and the
	// guild's current one (see MoodEntry.AnonymityThreshold). 0 on a day without answers.
	AnonymityThreshold int
	// Recipients is how many people the bot sent that day's survey to; nil when unknown.
	Recipients *int
}

// ReportMessage is an optional comment left with a mood, with its day. MemberDiscordID and
// Username are set only for a comment whose author is shown: the guild is not anonymous now
// and was not when it was written.
type ReportMessage struct {
	Day             time.Time
	Mood            Mood
	Message         string
	MemberDiscordID string
	Username        string
}

// Anonymous reports whether the comment hides its author.
func (m ReportMessage) Anonymous() bool { return m.MemberDiscordID == "" }

// WithheldDay reports whether a day with anonymous answers from this many people shows only
// how many answered: when they are fewer than the threshold k, anything more could point to
// them. Only the anonymous answers count, since the named ones are known anyway.
func WithheldDay(anonymousPeople, k int) bool {
	return anonymousPeople > 0 && anonymousPeople < max(k, MinAnonymityThreshold)
}

// BuildReport aggregates entries into a report, hiding identities when anonymous. An entry
// only shows its author when the guild is not anonymous now and was not when it was written.
//
// A day with anonymous answers from fewer people than the guild's threshold is withheld: it
// keeps how many answered, and nothing else from it counts anywhere, not in the period's
// average, distribution or comments. Leaving it out only of its own row would not be enough:
// the period's figures minus the other days' would give it back, and so would two reports
// whose periods differ by that day. When every answer falls on withheld days, the report is
// withheld too and keeps only the counts. usernames maps member Discord IDs to display names.
func BuildReport(guild Guild, from, to time.Time, entries []MoodEntry, usernames map[string]string) Report {
	k := max(guild.Settings.AnonymityThreshold, MinAnonymityThreshold)
	r := Report{
		GuildDiscordID:     guild.DiscordID,
		From:               from,
		To:                 to,
		Anonymous:          guild.Settings.Anonymous,
		AnonymityThreshold: k,
		Distribution:       map[Mood]int{},
	}
	for m := MoodAwful; m <= MoodGreat; m++ {
		r.Distribution[m] = 0
	}

	type acc struct {
		n, sum int
		// anonymous are who answered without their name showing, and k the highest threshold
		// among their answers (see MoodEntry.AnonymityThreshold).
		anonymous    map[string]struct{}
		k            int
		distribution map[Mood]int
		messages     []ReportMessage
	}
	perDay := map[time.Time]*acc{}
	people := map[string]struct{}{}
	for _, e := range entries {
		r.Responses++
		people[e.MemberDiscordID] = struct{}{}
		d := TruncateDay(e.Day)
		a := perDay[d]
		if a == nil {
			a = &acc{anonymous: map[string]struct{}{}, distribution: map[Mood]int{}}
			perDay[d] = a
		}
		a.n++
		a.sum += int(e.Mood)
		a.distribution[e.Mood]++
		shown := !guild.Settings.Anonymous && !e.Anonymous
		if !shown {
			a.anonymous[e.MemberDiscordID] = struct{}{}
			a.k = max(a.k, e.AnonymityThreshold)
		}
		if e.Message != "" {
			m := ReportMessage{Day: d, Mood: e.Mood, Message: e.Message}
			if shown {
				m.MemberDiscordID, m.Username = e.MemberDiscordID, usernames[e.MemberDiscordID]
			}
			a.messages = append(a.messages, m)
		}
	}
	r.Respondents = len(people)

	withheld := func(a *acc) bool { return WithheldDay(len(a.anonymous), max(k, a.k)) }
	// Days in order, so comments keep the order they were written in.
	days := make([]time.Time, 0, len(perDay))
	for d := range perDay {
		days = append(days, d)
	}
	slices.SortFunc(days, func(a, b time.Time) int { return a.Compare(b) })
	shown, total := 0, 0
	for _, d := range days {
		a := perDay[d]
		if withheld(a) {
			continue
		}
		shown += a.n
		total += a.sum
		for m, n := range a.distribution {
			r.Distribution[m] += n
		}
		r.Messages = append(r.Messages, a.messages...)
	}
	r.Withheld = r.Responses > 0 && shown == 0
	if shown > 0 {
		r.Average = float64(total) / float64(shown)
	}
	// Non-working days only show up when someone answered anyway.
	for d := TruncateDay(from); !d.After(TruncateDay(to)); d = d.AddDate(0, 0, 1) {
		a := perDay[d]
		if a == nil && !guild.Settings.IsWorkingDay(d) {
			continue
		}
		ds := DayStat{Day: d}
		if a != nil {
			ds.Responses = a.n
			ds.AnonymityThreshold = max(k, a.k)
			ds.Withheld = withheld(a)
			if ds.Withheld {
				// The report names the highest threshold among its withheld days: it is the one
				// no day reached when the whole report is withheld.
				r.AnonymityThreshold = max(r.AnonymityThreshold, ds.AnonymityThreshold)
			} else {
				ds.Average = float64(a.sum) / float64(a.n)
			}
		}
		r.Days = append(r.Days, ds)
	}
	return r
}
