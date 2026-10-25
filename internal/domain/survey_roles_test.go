package domain_test

import (
	"testing"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

func TestSurveyRolesReceives(t *testing.T) {
	const ext, staff, other = "400000000000000001", "400000000000000002", "400000000000000003"
	for _, c := range []struct {
		name  string
		rules domain.SurveyRoles
		roles []string
		want  bool
	}{
		{"everyone", domain.SurveyRoles{Mode: domain.SurveyEveryone}, []string{ext}, true},
		{"never set", domain.SurveyRoles{}, nil, true},
		{"except, without the role", domain.SurveyRoles{Mode: domain.SurveyExcept, RoleIDs: []string{ext}}, []string{other}, true},
		{"except, with the role", domain.SurveyRoles{Mode: domain.SurveyExcept, RoleIDs: []string{ext}}, []string{other, ext}, false},
		{"only, with the role", domain.SurveyRoles{Mode: domain.SurveyOnly, RoleIDs: []string{staff}}, []string{staff}, true},
		{"only, without roles", domain.SurveyRoles{Mode: domain.SurveyOnly, RoleIDs: []string{staff}}, nil, false},
	} {
		if got := c.rules.Receives(c.roles); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSurveyRolesValidate(t *testing.T) {
	const role = "400000000000000001"
	for name, c := range map[string]struct {
		rules domain.SurveyRoles
		ok    bool
	}{
		"everyone":            {domain.SurveyRoles{Mode: domain.SurveyEveryone}, true},
		"except one":          {domain.SurveyRoles{Mode: domain.SurveyExcept, RoleIDs: []string{role}}, true},
		"everyone with roles": {domain.SurveyRoles{Mode: domain.SurveyEveryone, RoleIDs: []string{role}}, false},
		"only without roles":  {domain.SurveyRoles{Mode: domain.SurveyOnly}, false},
		"unknown mode":        {domain.SurveyRoles{Mode: "some"}, false},
		"bad id":              {domain.SurveyRoles{Mode: domain.SurveyOnly, RoleIDs: []string{"abc"}}, false},
		"twice":               {domain.SurveyRoles{Mode: domain.SurveyOnly, RoleIDs: []string{role, role}}, false},
	} {
		if err := c.rules.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: err %v", name, err)
		}
	}
}
