package domain

import "testing"

func TestResolveAccess(t *testing.T) {
	const (
		guild, admin, other = "100000000000000001", "300000000000000001", "300000000000000002"
		staff, leads        = "400000000000000001", "400000000000000002"
	)
	g := Guild{DiscordID: guild, AdminDiscordID: admin}
	grants := []AccessGrant{
		{Kind: GrantRole, TargetID: staff, Level: AccessView},
		{Kind: GrantRole, TargetID: leads, Level: AccessManage},
		{Kind: GrantUser, TargetID: "300000000000000003", Level: AccessView},
	}
	cases := []struct {
		name    string
		user    string
		manages bool
		roles   []string
		want    AccessLevel
	}{
		{"main admin", admin, false, nil, AccessOwner},
		{"Discord administrator", other, true, nil, AccessOwner},
		{"plain member", other, false, []string{"400000000000000009"}, AccessNone},
		{"role with view", other, false, []string{staff}, AccessView},
		{"highest role wins", other, false, []string{staff, leads}, AccessManage},
		{"user grant", "300000000000000003", false, nil, AccessView},
		{"user grant plus role", "300000000000000003", false, []string{leads}, AccessManage},
		{"nobody", "", true, []string{leads}, AccessNone},
	}
	for _, c := range cases {
		if got := ResolveAccess(g, c.user, c.manages, c.roles, grants); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidateGrants(t *testing.T) {
	ok := AccessGrant{Kind: GrantRole, TargetID: "400000000000000001", Level: AccessManage}
	if err := ValidateGrants([]AccessGrant{ok, {Kind: GrantUser, TargetID: "400000000000000001", Level: AccessView}}); err != nil {
		t.Fatalf("a role and a user may share an ID: %v", err)
	}
	for name, gs := range map[string][]AccessGrant{
		"duplicate":   {ok, ok},
		"owner level": {{Kind: GrantUser, TargetID: "300000000000000001", Level: AccessOwner}},
		"bad kind":    {{Kind: "team", TargetID: "300000000000000001", Level: AccessView}},
		"bad id":      {{Kind: GrantUser, TargetID: "abc", Level: AccessView}},
	} {
		if err := ValidateGrants(gs); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
