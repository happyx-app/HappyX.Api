package domain

// SurveyRolesMode says how a guild's survey roles pick who receives the daily survey.
type SurveyRolesMode string

const (
	// SurveyEveryone sends the survey to every member who hasn't opted out. The default.
	SurveyEveryone SurveyRolesMode = "all"
	// SurveyExcept leaves out members with any of the roles, e.g. external collaborators.
	SurveyExcept SurveyRolesMode = "except"
	// SurveyOnly sends it only to members with at least one of the roles.
	SurveyOnly SurveyRolesMode = "only"
)

// SurveyRoles narrows who receives the daily survey by Discord role. Members who opted out
// never receive it, whatever their roles.
type SurveyRoles struct {
	Mode    SurveyRolesMode
	RoleIDs []string // empty with SurveyEveryone
}

// MaxSurveyRoles keeps the list short enough to read at a glance.
const MaxSurveyRoles = 50

// Validate accepts an empty mode as SurveyEveryone, for settings that never set it.
func (s SurveyRoles) Validate() error {
	switch s.Mode {
	case SurveyEveryone, "":
		if len(s.RoleIDs) > 0 {
			return invalid("survey_roles.role_ids must be empty when mode is %q", SurveyEveryone)
		}
		return nil
	case SurveyExcept, SurveyOnly:
	default:
		return invalid("survey_roles.mode must be %q, %q or %q", SurveyEveryone, SurveyExcept, SurveyOnly)
	}
	if len(s.RoleIDs) == 0 {
		return invalid("survey_roles.role_ids must have at least one role when mode is %q", s.Mode)
	}
	if len(s.RoleIDs) > MaxSurveyRoles {
		return invalid("survey_roles.role_ids can have at most %d roles", MaxSurveyRoles)
	}
	seen := map[string]bool{}
	for _, id := range s.RoleIDs {
		if err := validateSnowflake("role id", id); err != nil {
			return err
		}
		if seen[id] {
			return invalid("role %s is listed twice", id)
		}
		seen[id] = true
	}
	return nil
}

// Receives reports whether a member with roleIDs gets the survey. Being left out wins: with
// SurveyExcept, one excluded role is enough whatever other roles the member has.
func (s SurveyRoles) Receives(roleIDs []string) bool {
	if s.Mode != SurveyExcept && s.Mode != SurveyOnly {
		return true
	}
	listed := map[string]bool{}
	for _, id := range s.RoleIDs {
		listed[id] = true
	}
	for _, id := range roleIDs {
		if listed[id] {
			return s.Mode == SurveyOnly
		}
	}
	return s.Mode == SurveyExcept
}
