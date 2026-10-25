package domain_test

import (
	"testing"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

func TestRoleFilterIncludes(t *testing.T) {
	const ext, staff, other = "400000000000000001", "400000000000000002", "400000000000000003"
	for _, c := range []struct {
		name  string
		rules domain.RoleFilter
		roles []string
		want  bool
	}{
		{"everyone", domain.RoleFilter{Mode: domain.RolesEveryone}, []string{ext}, true},
		{"never set", domain.RoleFilter{}, nil, true},
		{"except, without the role", domain.RoleFilter{Mode: domain.RolesExcept, RoleIDs: []string{ext}}, []string{other}, true},
		{"except, with the role", domain.RoleFilter{Mode: domain.RolesExcept, RoleIDs: []string{ext}}, []string{other, ext}, false},
		{"only, with the role", domain.RoleFilter{Mode: domain.RolesOnly, RoleIDs: []string{staff}}, []string{staff}, true},
		{"only, without roles", domain.RoleFilter{Mode: domain.RolesOnly, RoleIDs: []string{staff}}, nil, false},
	} {
		if got := c.rules.Includes(c.roles); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRoleFilterValidate(t *testing.T) {
	const role = "400000000000000001"
	for name, c := range map[string]struct {
		rules domain.RoleFilter
		ok    bool
	}{
		"everyone":            {domain.RoleFilter{Mode: domain.RolesEveryone}, true},
		"except one":          {domain.RoleFilter{Mode: domain.RolesExcept, RoleIDs: []string{role}}, true},
		"everyone with roles": {domain.RoleFilter{Mode: domain.RolesEveryone, RoleIDs: []string{role}}, false},
		"only without roles":  {domain.RoleFilter{Mode: domain.RolesOnly}, false},
		"unknown mode":        {domain.RoleFilter{Mode: "some"}, false},
		"bad id":              {domain.RoleFilter{Mode: domain.RolesOnly, RoleIDs: []string{"abc"}}, false},
		"twice":               {domain.RoleFilter{Mode: domain.RolesOnly, RoleIDs: []string{role, role}}, false},
	} {
		if err := c.rules.Validate("survey_roles"); (err == nil) != c.ok {
			t.Errorf("%s: err %v", name, err)
		}
	}
}
