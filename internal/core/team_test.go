package core

import (
	"strings"
	"testing"
)

func validTeam() Team {
	return Team{
		ID:        "team-1",
		ClubID:    "club-1",
		Name:      "1st XV",
		ShortName: "1XV",
		Active:    true,
	}
}

func TestTeamValidateAcceptsAValidTeam(t *testing.T) {
	if err := validTeam().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestTeamValidateAllowsEmptyShortName(t *testing.T) {
	team := validTeam()
	team.ShortName = ""

	if err := team.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil — short name is optional", err)
	}
}

func TestTeamValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Team)
		field  string
	}{
		{"empty name", func(tm *Team) { tm.Name = "   " }, "name"},
		{"name too long", func(tm *Team) { tm.Name = strings.Repeat("a", 61) }, "name"},
		{"short name too long", func(tm *Team) { tm.ShortName = "TOOLONGSHORT" }, "shortName"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			team := validTeam()
			c.mutate(&team)

			err := team.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestTeamDisplayShortNameFallsBackToName(t *testing.T) {
	team := validTeam()
	if got := team.Display(); got != "1XV" {
		t.Errorf("Display() = %q, want the short name", got)
	}

	team.ShortName = ""
	if got := team.Display(); got != "1st XV" {
		t.Errorf("Display() = %q, want the full name when no short name is set", got)
	}
}
