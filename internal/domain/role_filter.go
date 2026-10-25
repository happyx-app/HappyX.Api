package domain

// RoleFilterMode says how a RoleFilter's roles pick who takes part.
type RoleFilterMode string

const (
	// RolesEveryone lets every member who hasn't opted out take part. The default.
	RolesEveryone RoleFilterMode = "all"
	// RolesExcept leaves out members with any of the roles, e.g. external collaborators.
	RolesExcept RoleFilterMode = "except"
	// RolesOnly lets in only members with at least one of the roles.
	RolesOnly RoleFilterMode = "only"
)

// RoleFilter narrows by Discord role who receives the daily survey (a guild's survey_roles) or
// plays the game (game_roles). Members who opted out never take part, whatever their roles.
type RoleFilter struct {
	Mode    RoleFilterMode
	RoleIDs []string // empty with RolesEveryone
}

// MaxFilterRoles keeps a list short enough to read at a glance.
const MaxFilterRoles = 50

// Validate checks the filter stored in field, e.g. "survey_roles". It accepts an empty mode as
// RolesEveryone, for settings that never set it.
func (f RoleFilter) Validate(field string) error {
	switch f.Mode {
	case RolesEveryone, "":
		if len(f.RoleIDs) > 0 {
			return invalid("%s.role_ids must be empty when mode is %q", field, RolesEveryone)
		}
		return nil
	case RolesExcept, RolesOnly:
	default:
		return invalid("%s.mode must be %q, %q or %q", field, RolesEveryone, RolesExcept, RolesOnly)
	}
	if len(f.RoleIDs) == 0 {
		return invalid("%s.role_ids must have at least one role when mode is %q", field, f.Mode)
	}
	if len(f.RoleIDs) > MaxFilterRoles {
		return invalid("%s.role_ids can have at most %d roles", field, MaxFilterRoles)
	}
	seen := map[string]bool{}
	for _, id := range f.RoleIDs {
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

// Includes reports whether a member with roleIDs takes part. Being left out wins: with
// RolesExcept, one excluded role is enough whatever other roles the member has.
func (f RoleFilter) Includes(roleIDs []string) bool {
	if f.Mode != RolesExcept && f.Mode != RolesOnly {
		return true
	}
	listed := map[string]bool{}
	for _, id := range f.RoleIDs {
		listed[id] = true
	}
	for _, id := range roleIDs {
		if listed[id] {
			return f.Mode == RolesOnly
		}
	}
	return f.Mode == RolesExcept
}
