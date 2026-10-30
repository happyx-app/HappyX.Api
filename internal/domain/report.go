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
	Responses      int
	Respondents    int
	Average        float64      // 0 when there are no responses
	Distribution   map[Mood]int // count per mood
	Days           []DayStat
	Messages       []ReportMessage
	Kudos          KudosSummary
	// Participation compares answers with surveys sent, on the days the bot reported how many
	// people it sent the survey to. Nil when it reported none in the period.
	Participation *Participation
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
	Average   float64
	// Recipients is how many people the bot sent that day's survey to; nil when unknown.
	Recipients *int
}

// ReportMessage is an optional comment left with a mood. MemberDiscordID is empty when the
// guild is anonymous, or was when the comment was written.
type ReportMessage struct {
	Day             time.Time
	Mood            Mood
	Message         string
	MemberDiscordID string
	Username        string
}

// BuildReport aggregates entries into a report, hiding identities when anonymous. An entry
// only shows its author when the guild is not anonymous now and was not when it was written.
// usernames maps member Discord IDs to display names.
func BuildReport(guild Guild, from, to time.Time, entries []MoodEntry, usernames map[string]string) Report {
	r := Report{
		GuildDiscordID: guild.DiscordID,
		From:           from,
		To:             to,
		Anonymous:      guild.Settings.Anonymous,
		Distribution:   map[Mood]int{},
	}
	for m := MoodAwful; m <= MoodGreat; m++ {
		r.Distribution[m] = 0
	}

	type acc struct{ n, sum int }
	perDay := map[time.Time]*acc{}
	people := map[string]struct{}{}
	total := 0
	for _, e := range entries {
		r.Responses++
		total += int(e.Mood)
		r.Distribution[e.Mood]++
		people[e.MemberDiscordID] = struct{}{}
		d := TruncateDay(e.Day)
		if perDay[d] == nil {
			perDay[d] = &acc{}
		}
		perDay[d].n++
		perDay[d].sum += int(e.Mood)
		if e.Message != "" {
			msg := ReportMessage{Day: d, Mood: e.Mood, Message: e.Message}
			if !guild.Settings.Anonymous && !e.Anonymous {
				msg.MemberDiscordID = e.MemberDiscordID
				msg.Username = usernames[e.MemberDiscordID]
			}
			r.Messages = append(r.Messages, msg)
		}
	}
	r.Respondents = len(people)
	if r.Responses > 0 {
		r.Average = float64(total) / float64(r.Responses)
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
			ds.Average = float64(a.sum) / float64(a.n)
		}
		r.Days = append(r.Days, ds)
	}
	return r
}
