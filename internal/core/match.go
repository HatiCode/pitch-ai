package core

import (
	"strconv"
	"strings"
	"time"
)

type Venue string

const (
	VenueHome    Venue = "home"
	VenueAway    Venue = "away"
	VenueNeutral Venue = "neutral"
)

type MatchStatus string

const (
	MatchScheduled  MatchStatus = "scheduled"
	MatchInProgress MatchStatus = "in_progress"
	MatchCompleted  MatchStatus = "completed"
)

// AllVenues returns every venue, for building pickers and generated types.
func AllVenues() []Venue {
	return []Venue{VenueHome, VenueAway, VenueNeutral}
}

// AllMatchStatuses returns every match status in lifecycle order.
func AllMatchStatuses() []MatchStatus {
	return []MatchStatus{MatchScheduled, MatchInProgress, MatchCompleted}
}

// Jersey ranges for a match squad: 1-15 start, 16-23 are replacements.
const (
	firstStarterJersey = 1
	lastStarterJersey  = 15
	firstBenchJersey   = 16
	lastBenchJersey    = 23
)

// LineupSlot ties a jersey number to the player wearing it.
type LineupSlot struct {
	Jersey   int    `json:"jersey" firestore:"jersey"`
	PlayerID string `json:"playerId" firestore:"playerId"`
}

// Lineup is the selected squad for one match.
type Lineup struct {
	Starters []LineupSlot `json:"starters" firestore:"starters"`
	Bench    []LineupSlot `json:"bench" firestore:"bench"`
}

func (l Lineup) IsEmpty() bool {
	return len(l.Starters) == 0 && len(l.Bench) == 0
}

// Validate checks the lineup is a legal selection from the eligible squad: one
// jersey per player, one player per jersey, and every starting jersey filled.
//
// Selecting the same player twice, or someone outside the squad, are the two
// mistakes that silently corrupt every minutes-played figure downstream — so
// both are rejected here rather than discovered in a season report.
func (l Lineup) Validate(eligible map[string]bool) error {
	seenJersey := map[int]bool{}
	seenPlayer := map[string]bool{}

	check := func(slots []LineupSlot, field string, low, high int) error {
		for _, slot := range slots {
			if slot.Jersey < low || slot.Jersey > high {
				return ValidationError{
					Field:  field,
					Reason: "jersey " + strconv.Itoa(slot.Jersey) + " is outside " + strconv.Itoa(low) + "-" + strconv.Itoa(high),
				}
			}
			if seenJersey[slot.Jersey] {
				return ValidationError{Field: field, Reason: "jersey " + strconv.Itoa(slot.Jersey) + " is assigned twice"}
			}
			seenJersey[slot.Jersey] = true

			if slot.PlayerID == "" {
				return ValidationError{Field: field, Reason: "jersey " + strconv.Itoa(slot.Jersey) + " has no player"}
			}
			if seenPlayer[slot.PlayerID] {
				return ValidationError{Field: field, Reason: "player " + slot.PlayerID + " is selected twice"}
			}
			seenPlayer[slot.PlayerID] = true

			if !eligible[slot.PlayerID] {
				return ValidationError{Field: field, Reason: "player " + slot.PlayerID + " is not in this squad"}
			}
		}
		return nil
	}

	if err := check(l.Starters, "starters", firstStarterJersey, lastStarterJersey); err != nil {
		return err
	}
	if err := check(l.Bench, "bench", firstBenchJersey, lastBenchJersey); err != nil {
		return err
	}

	for jersey := firstStarterJersey; jersey <= lastStarterJersey; jersey++ {
		if !seenJersey[jersey] {
			return ValidationError{Field: "starters", Reason: "jersey " + strconv.Itoa(jersey) + " is unfilled"}
		}
	}
	return nil
}

type Match struct {
	ID          string      `json:"id,omitempty" firestore:"-"`
	ClubID      string      `json:"clubId,omitempty" firestore:"-"`
	TeamID      string      `json:"teamId,omitempty" firestore:"-"`
	SeasonID    string      `json:"seasonId" firestore:"seasonId"`
	Opponent    string      `json:"opponent" firestore:"opponent"`
	KickoffAt   time.Time   `json:"kickoffAt" firestore:"kickoffAt"`
	Competition string      `json:"competition" firestore:"competition"`
	Venue       Venue       `json:"venue" firestore:"venue"`
	Status      MatchStatus `json:"status" firestore:"status"`
	Lineup      Lineup      `json:"lineup" firestore:"lineup"`
}

func (m Match) Validate() error {
	if strings.TrimSpace(m.Opponent) == "" {
		return ValidationError{Field: "opponent", Reason: "must not be empty"}
	}
	if m.KickoffAt.IsZero() {
		return ValidationError{Field: "kickoffAt", Reason: "must be set"}
	}
	if strings.TrimSpace(m.SeasonID) == "" {
		return ValidationError{Field: "seasonId", Reason: "must not be empty"}
	}
	switch m.Venue {
	case VenueHome, VenueAway, VenueNeutral:
	default:
		return ValidationError{Field: "venue", Reason: "must be home, away or neutral"}
	}
	switch m.Status {
	case MatchScheduled, MatchInProgress, MatchCompleted:
	default:
		return ValidationError{Field: "status", Reason: "must be scheduled, in_progress or completed"}
	}
	return nil
}
