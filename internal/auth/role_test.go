package auth

import "testing"

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role Role
		act  Action
		want bool
	}{
		{RoleCoach, ActionRead, true},
		{RoleCoach, ActionTagMatch, true},
		{RoleCoach, ActionManageSquad, false},
		{RoleCoach, ActionEditMatch, true},
		{RoleAnalyst, ActionRead, true},
		{RoleAnalyst, ActionEditMatch, true},
		{RoleAnalyst, ActionTagMatch, false},
		{RoleAnalyst, ActionManageSquad, false},
		{RoleAdmin, ActionManageSquad, true},
		{RoleAdmin, ActionTagMatch, true},
		{RoleAdmin, ActionEditMatch, true},
		{Role("nonsense"), ActionRead, false},
	}

	for _, c := range cases {
		if got := c.role.Can(c.act); got != c.want {
			t.Errorf("Role(%q).Can(%q) = %v, want %v", c.role, c.act, got, c.want)
		}
	}
}
