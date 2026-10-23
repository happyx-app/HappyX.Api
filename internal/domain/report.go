package domain

import "time"

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
}

// KudosSummary counts the kudos given in a report's period. Kudos are public, so they keep
// their names even when the guild's moods are anonymous.
type KudosSummary struct {
	Count     int
	Givers    int
	Receivers int
	// Highlights are the latest kudos, newest first, at most MaxKudosHighlights.
	Highlights []Kudos
}

const MaxKudosHighlights = 5

// SummarizeKudos counts kudos (ordered oldest first) and keeps the latest as highlights.
func SummarizeKudos(kudos []Kudos) KudosSummary {
	s := KudosSummary{Count: len(kudos), Highlights: []Kudos{}}
	givers, receivers := map[string]struct{}{}, map[string]struct{}{}
	for _, k := range kudos {
		givers[k.GiverDiscordID] = struct{}{}
		receivers[k.ReceiverDiscordID] = struct{}{}
	}
	s.Givers, s.Receivers = len(givers), len(receivers)
	for i := len(kudos) - 1; i >= 0 && len(s.Highlights) < MaxKudosHighlights; i-- {
		s.Highlights = append(s.Highlights, kudos[i])
	}
	return s
}

type DayStat struct {
	Day       time.Time
	Responses int
	Average   float64
}

// ReportMessage is an optional comment left with a mood. MemberDiscordID is empty when the
// guild is anonymous.
type ReportMessage struct {
	Day             time.Time
	Mood            Mood
	Message         string
	MemberDiscordID string
	Username        string
}

// BuildReport aggregates entries into a report, hiding identities when anonymous.
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
			if !guild.Settings.Anonymous {
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
	for d := TruncateDay(from); !d.After(TruncateDay(to)); d = d.AddDate(0, 0, 1) {
		ds := DayStat{Day: d}
		if a := perDay[d]; a != nil {
			ds.Responses = a.n
			ds.Average = float64(a.sum) / float64(a.n)
		}
		r.Days = append(r.Days, ds)
	}
	return r
}
