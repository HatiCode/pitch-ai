package auth

type Role string

const (
	RoleCoach   Role = "coach"
	RoleAnalyst Role = "analyst"
	RoleAdmin   Role = "admin"
)

type Action string

const (
	ActionRead        Action = "read"
	ActionTagMatch    Action = "tag_match"
	ActionEditMatch   Action = "edit_match"
	ActionManageSquad Action = "manage_squad"
)

// AllRoles returns every role, least to most privileged.
func AllRoles() []Role {
	return []Role{RoleCoach, RoleAnalyst, RoleAdmin}
}

var rolePermissions = map[Role]map[Action]bool{
	RoleCoach: {
		ActionRead:      true,
		ActionTagMatch:  true,
		ActionEditMatch: true,
	},
	RoleAnalyst: {
		ActionRead:      true,
		ActionEditMatch: true,
	},
	RoleAdmin: {
		ActionRead:        true,
		ActionTagMatch:    true,
		ActionEditMatch:   true,
		ActionManageSquad: true,
	},
}

// Can reports whether the role permits the action. Unknown roles permit nothing.
func (r Role) Can(a Action) bool {
	return rolePermissions[r][a]
}
