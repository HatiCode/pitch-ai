package core

import (
	"strings"
	"testing"
)

func validPlayer() Player {
	return Player{
		ID:        "player-1",
		ClubID:    "club-1",
		FirstName: "Thomas",
		LastName:  "Lefevre",
		DOB:       "1998-04-12",
		Positions: []Position{Openside},
		TeamIDs:   []string{"team-1"},
		Status:    PlayerActive,
	}
}

func TestPlayerValidateAcceptsAValidPlayer(t *testing.T) {
	if err := validPlayer().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestPlayerValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Player)
		field  string
	}{
		{"empty first name", func(p *Player) { p.FirstName = "  " }, "firstName"},
		{"empty last name", func(p *Player) { p.LastName = "" }, "lastName"},
		{"malformed dob", func(p *Player) { p.DOB = "12/04/1998" }, "dob"},
		{"impossible dob", func(p *Player) { p.DOB = "1998-13-45" }, "dob"},
		{"no positions", func(p *Player) { p.Positions = nil }, "positions"},
		{"unknown position", func(p *Player) { p.Positions = []Position{"prop"} }, "positions"},
		{"group used as position", func(p *Player) { p.Positions = []Position{"back_row"} }, "positions"},
		{"unknown status", func(p *Player) { p.Status = "retired-ish" }, "status"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validPlayer()
			c.mutate(&p)

			err := p.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestPlayerValidateAllowsEmptyDOB(t *testing.T) {
	p := validPlayer()
	p.DOB = ""

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil for an unrecorded date of birth", err)
	}
}

func TestPlayerDisplayName(t *testing.T) {
	if got, want := validPlayer().DisplayName(), "T. Lefevre"; got != want {
		t.Errorf("DisplayName() = %q, want %q", got, want)
	}
}

func TestPlayerGroupsDeduplicates(t *testing.T) {
	p := validPlayer()
	p.Positions = []Position{Blindside, Openside, ScrumHalf}

	groups := p.Groups()
	if len(groups) != 2 {
		t.Fatalf("Groups() = %v, want 2 groups (back_row, half_backs)", groups)
	}
	if groups[0] != BackRow || groups[1] != HalfBacks {
		t.Errorf("Groups() = %v, want [back_row half_backs] in position order", groups)
	}
}
