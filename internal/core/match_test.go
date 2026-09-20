package core

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func fullStarters() []LineupSlot {
	slots := make([]LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		slots = append(slots, LineupSlot{Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey)})
	}
	return slots
}

func eligibleSquad() map[string]bool {
	squad := map[string]bool{}
	for jersey := 1; jersey <= 23; jersey++ {
		squad["player-"+strconv.Itoa(jersey)] = true
	}
	return squad
}

func validMatch() Match {
	return Match{
		ID:          "match-1",
		ClubID:      "club-1",
		TeamID:      "team-1",
		SeasonID:    "2026-27",
		Opponent:    "Castelnau RC",
		KickoffAt:   time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Competition: "Regional 1",
		Venue:       VenueHome,
		Status:      MatchScheduled,
	}
}

func TestMatchValidateAcceptsAValidMatch(t *testing.T) {
	if err := validMatch().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestMatchValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Match)
		field  string
	}{
		{"no opponent", func(m *Match) { m.Opponent = " " }, "opponent"},
		{"no kickoff", func(m *Match) { m.KickoffAt = time.Time{} }, "kickoffAt"},
		{"unknown venue", func(m *Match) { m.Venue = "the moon" }, "venue"},
		{"unknown status", func(m *Match) { m.Status = "abandoned-ish" }, "status"},
		{"no season", func(m *Match) { m.SeasonID = "" }, "seasonId"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := validMatch()
			c.mutate(&m)

			err := m.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestLineupIsEmpty(t *testing.T) {
	if !(Lineup{}).IsEmpty() {
		t.Error("zero Lineup should be empty")
	}
	if (Lineup{Starters: fullStarters()}).IsEmpty() {
		t.Error("Lineup with starters should not be empty")
	}
}

func TestLineupValidateAcceptsAFullXV(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 16, PlayerID: "player-16"}, {Jersey: 17, PlayerID: "player-17"}},
	}

	if err := lineup.Validate(eligibleSquad()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestLineupValidateAllowsAnEmptyBench(t *testing.T) {
	if err := (Lineup{Starters: fullStarters()}).Validate(eligibleSquad()); err != nil {
		t.Errorf("Validate() error = %v, want nil — replacements are optional", err)
	}
}

func TestLineupValidateRequiresAllFifteenJerseys(t *testing.T) {
	lineup := Lineup{Starters: fullStarters()[:14]}

	err := lineup.Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a 14-man starting lineup")
	}
	if !strings.Contains(err.Error(), "starters") {
		t.Errorf("error = %q, want it to name the starters field", err)
	}
}

func TestLineupValidateRejectsDuplicateJersey(t *testing.T) {
	starters := fullStarters()
	starters[14].Jersey = 1 // two players wearing 1, and nobody wearing 15

	if err := (Lineup{Starters: starters}).Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for a duplicated jersey")
	}
}

func TestLineupValidateRejectsSamePlayerTwice(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 16, PlayerID: "player-7"}},
	}

	err := lineup.Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a player named twice")
	}
	if !strings.Contains(err.Error(), "player-7") {
		t.Errorf("error = %q, want it to name the duplicated player", err)
	}
}

func TestLineupValidateRejectsStarterJerseyOutOfRange(t *testing.T) {
	starters := fullStarters()
	starters[0].Jersey = 16 // a replacement number in the starting XV

	if err := (Lineup{Starters: starters}).Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for jersey 16 among the starters")
	}
}

func TestLineupValidateRejectsBenchJerseyOutOfRange(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 24, PlayerID: "player-16"}},
	}

	if err := lineup.Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for bench jersey 24")
	}
}

func TestLineupValidateRejectsEmptyPlayerID(t *testing.T) {
	starters := fullStarters()
	starters[3].PlayerID = ""

	if err := (Lineup{Starters: starters}).Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for an unfilled jersey")
	}
}

func TestLineupValidateRejectsIneligiblePlayer(t *testing.T) {
	starters := fullStarters()
	starters[0].PlayerID = "player-from-another-club"

	err := (Lineup{Starters: starters}).Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a player outside the squad")
	}
	if !strings.Contains(err.Error(), "player-from-another-club") {
		t.Errorf("error = %q, want it to name the ineligible player", err)
	}
}
