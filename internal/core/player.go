package core

import (
	"slices"
	"strings"
	"time"
)

type PlayerStatus string

const (
	PlayerActive   PlayerStatus = "active"
	PlayerInactive PlayerStatus = "inactive"
)

type Player struct {
	ID        string       `json:"id,omitempty" firestore:"-"`
	ClubID    string       `json:"clubId,omitempty" firestore:"-"`
	FirstName string       `json:"firstName" firestore:"firstName"`
	LastName  string       `json:"lastName" firestore:"lastName"`
	DOB       string       `json:"dob" firestore:"dob"`
	Positions []Position   `json:"positions" firestore:"positions"`
	TeamIDs   []string     `json:"teamIds" firestore:"teamIds"`
	Status    PlayerStatus `json:"status" firestore:"status"`
}

// DisplayName renders the shirt-back form: initial and surname.
func (p Player) DisplayName() string {
	first := strings.TrimSpace(p.FirstName)
	if first == "" {
		return strings.TrimSpace(p.LastName)
	}
	return string([]rune(first)[0]) + ". " + strings.TrimSpace(p.LastName)
}

// Groups returns the distinct comparison groups this player covers, in the
// order their positions appear. A player who covers 6 and 7 belongs to one
// group; one who covers 7 and 9 belongs to two.
func (p Player) Groups() []PositionGroup {
	var groups []PositionGroup
	for _, position := range p.Positions {
		group := position.Group()
		if group != "" && !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	return groups
}

// Validate checks the fields a caller supplies. ID and ClubID are assigned by
// the service layer and are deliberately not checked here.
func (p Player) Validate() error {
	if strings.TrimSpace(p.FirstName) == "" {
		return ValidationError{Field: "firstName", Reason: "must not be empty"}
	}
	if strings.TrimSpace(p.LastName) == "" {
		return ValidationError{Field: "lastName", Reason: "must not be empty"}
	}
	if p.DOB != "" {
		if _, err := time.Parse(time.DateOnly, p.DOB); err != nil {
			return ValidationError{Field: "dob", Reason: "must be a real date in YYYY-MM-DD form"}
		}
	}
	if len(p.Positions) == 0 {
		return ValidationError{Field: "positions", Reason: "at least one position is required"}
	}
	for _, position := range p.Positions {
		if !position.Valid() {
			return ValidationError{Field: "positions", Reason: "unknown position " + string(position)}
		}
	}
	switch p.Status {
	case PlayerActive, PlayerInactive:
	default:
		return ValidationError{Field: "status", Reason: "must be active or inactive"}
	}
	return nil
}
