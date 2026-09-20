package core

import "strings"

const (
	maxTeamNameLength      = 60
	maxTeamShortNameLength = 8
)

// Team is a squad that plays fixtures — the 1st XV, the Colts, the women's
// side. Players belong to the club, not to a team, so one player can be in
// several teams at once.
type Team struct {
	ID        string `json:"id,omitempty" firestore:"-"`
	ClubID    string `json:"clubId,omitempty" firestore:"-"`
	Name      string `json:"name" firestore:"name"`
	ShortName string `json:"shortName" firestore:"shortName"`
	Active    bool   `json:"active" firestore:"active"`
}

// Display is the compact label for navigation and match headers.
func (t Team) Display() string {
	if short := strings.TrimSpace(t.ShortName); short != "" {
		return short
	}
	return strings.TrimSpace(t.Name)
}

func (t Team) Validate() error {
	name := strings.TrimSpace(t.Name)
	if name == "" {
		return ValidationError{Field: "name", Reason: "must not be empty"}
	}
	if len(name) > maxTeamNameLength {
		return ValidationError{Field: "name", Reason: "must be 60 characters or fewer"}
	}
	if len(strings.TrimSpace(t.ShortName)) > maxTeamShortNameLength {
		return ValidationError{Field: "shortName", Reason: "must be 8 characters or fewer"}
	}
	return nil
}
