package domain

import "time"

// The owner's admin dashboard sees every guild, but only as counts: never a mood, a message, a
// username or anything else a guild's members wrote. These are its read models.

// IdleAfter is how long a guild with the bot can go without answers before it counts as idle.
const IdleAfter = 14 * 24 * time.Hour

type AdminGuildStatus string

const (
	AdminGuildActive AdminGuildStatus = "active"
	AdminGuildIdle   AdminGuildStatus = "idle" // the bot is in, but nobody answered for IdleAfter
	AdminGuildLeft   AdminGuildStatus = "left" // the bot was removed; kept until PurgeAt
)

// AdminGuild is one guild as the admin dashboard lists it.
type AdminGuild struct {
	DiscordID  string
	Name       string
	Status     AdminGuildStatus
	CreatedAt  time.Time
	LeftAt     *time.Time
	PurgeAt    *time.Time // when a guild the bot left is deleted
	Language   string
	Timezone   string
	Members    int // still in the server
	OptedOut   int // in the server, said «don't ask me again»
	Active7d   int // answered at least once in the last 7 days
	LastAnswer *time.Time
	// Recipients and Answered are the last 7 days' surveys sent and the answers to them, for
	// the response rate. Surveys sent before the bot reported recipients are left out.
	Recipients int
	Answered   int
	Failed7d   int // DMs that failed in the last 7 days, any kind
}

// GuildStatus says whether a guild is active, idle or left, as of now.
func GuildStatus(active bool, createdAt time.Time, lastAnswer *time.Time, now time.Time) AdminGuildStatus {
	if !active {
		return AdminGuildLeft
	}
	cutoff := now.Add(-IdleAfter)
	if createdAt.Before(cutoff) && (lastAnswer == nil || lastAnswer.Before(TruncateDay(cutoff))) {
		return AdminGuildIdle
	}
	return AdminGuildActive
}

// AdminWeek is one ISO week (starting Monday) of a trend.
type AdminWeek struct {
	Week       time.Time
	Recipients int
	Answered   int // answers to surveys with a known number of recipients
	Answers    int // all answers
	Joins      int // guilds that added the bot
	Leaves     int // guilds that removed the bot or were deleted from the dashboard
	Guilds     int // guilds with the bot at the end of the week
}

type AdminGuildSettings struct {
	Anonymous       bool
	SurveyHour      int
	WorkingDays     Weekdays
	ReportFrequency ReportFrequency
	ReportDM        bool
	CustomMessage   bool // only whether there is one, never its text
	SurveyRoles     RoleFilterMode
	GameRoles       RoleFilterMode
	GamesEnabled    bool
	KudosChannel    bool
}

// AdminGuildDetail is one guild's page on the admin dashboard.
type AdminGuildDetail struct {
	AdminGuild
	Settings     AdminGuildSettings
	Weeks        []AdminWeek // the last 8, oldest first; only Recipients, Answered and Answers
	Kudos7d      int
	Answers30d   int
	WithMessage  int // of Answers30d, how many had a message; never the text
	LastReport   *time.Time
	GameAnswers7 int
}

// AdminDay is one day of the trend of every guild together.
type AdminDay struct {
	Day         time.Time
	Answers     int
	WithMessage int
	Recipients  int
	Answered    int // answers to surveys with a known number of recipients
	Failed      int // survey DMs that failed
}

type AdminOverview struct {
	Guilds struct {
		WithBot  int // the bot is in: active + idle
		Idle     int
		Left     int // removed the bot, kept for LeftGuildRetention
		Deleted  int // deleted for good, ever: from the dashboard, or LeftGuildRetention after leaving
		NewWeek  int // added the bot in the last 7 days
		LeftWeek int
	}
	People struct {
		Members  int // distinct people in guilds with the bot
		OptedOut int // member rows that said «don't ask me again»
		Today    int // distinct people who answered today
		Week     int // ... in the last 7 days
		Month    int // ... in the last 30 days
		Repeat   int // of Week, those who answered on 3 or more days
	}
	Usage struct {
		Answers7d     int
		WithMessage7d int
		Kudos7d       int
		GamesOn       int
		Proposals7d   int
		GameAnswers7d int
	}
	// Phase0 are the numbers that decide whether paid plans are worth building.
	Phase0 struct {
		ActiveGuilds int // guilds where FreeLimitActive or more people answered in the last 7 days
		OverFree     int // guilds whose survey reaches more than FreePlanPeople
	}
	Failed7d map[DispatchKind]int
	Days     []AdminDay  // oldest first
	Weeks    []AdminWeek // the last 12, oldest first
}

// AdminDeliveryDay counts one day's failed DMs by kind.
type AdminDeliveryDay struct {
	Day    time.Time
	Sent   map[DispatchKind]int
	Failed map[DispatchKind]int
}

type AdminDatabase struct {
	Version        string
	SizeBytes      int64
	LargestTable   string
	LargestBytes   int64
	Connections    int
	MaxConnections int
	Migration      int
}

// Count is how many guilds have one value of a setting.
type Count struct {
	Value string
	N     int
}

// AdminSettings is how the guilds with the bot have configured HappyX.
type AdminSettings struct {
	Guilds          int
	Language        []Count
	Timezone        []Count
	SurveyHour      []Count
	Anonymous       []Count
	ReportFrequency []Count
	ReportDM        []Count
	SurveyRoles     []Count
	Extras          []Count // games, kudos_channel, custom_message, custom_days
}

// Backup is one database backup file.
type Backup struct {
	Name     string
	Size     int64
	Modified time.Time
}
