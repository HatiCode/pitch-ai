package core

import (
	"slices"
	"testing"
)

// The catalogue is configuration, so the test spells out what it should contain
// rather than deriving it from the code under test. Spec section 6 lists these
// kinds by name; a kind added to the catalogue without a decision recorded here
// should fail rather than quietly reach a coach's thumb.
var expectedGroups = map[EventGroup][]EventKind{
	GroupPlay: {
		KindTackleMade, KindTackleMissed, KindCarry, KindLinebreak,
		KindDefenderBeaten, KindOffload, KindClearout, KindJackalWon,
		KindTurnoverWon, KindHandlingError, KindKick, KindTry, KindSub,
	},
	GroupSetPiece: {
		KindScrumWon, KindScrumLost, KindScrumPenaltyWon, KindScrumPenaltyConceded,
		KindLineoutWon, KindLineoutLost, KindThrowNotStraight, KindMaulFromLineout,
		KindRestartReceived, KindRestartLost,
	},
	GroupDiscipline: {
		KindOffside, KindRuckOffence, KindHighTackle, KindNotReleasing,
		KindScrumOffence, KindFoulPlay, KindYellowCard, KindRedCard,
	},
	GroupScore: {
		KindConversionMade, KindConversionMissed, KindPenaltyGoal, KindDropGoal,
		KindOppositionTry, KindOppositionConversion, KindOppositionPenalty,
		KindOppositionDrop,
	},
	GroupControl: {
		KindPeriodStarted, KindClockPaused, KindClockResumed, KindPeriodEnded,
		KindMatchEnded, KindZoneChanged, KindVoid,
	},
}

func kindsInGroup(group EventGroup) []EventKind {
	var kinds []EventKind
	for _, entry := range Catalogue() {
		if entry.Group == group {
			kinds = append(kinds, entry.Kind)
		}
	}
	return kinds
}

func TestCatalogueGroupsHoldExactlyTheExpectedKinds(t *testing.T) {
	for group, want := range expectedGroups {
		t.Run(string(group), func(t *testing.T) {
			got := kindsInGroup(group)
			if !slices.Equal(got, want) {
				t.Errorf("kinds in %q =\n  %v\nwant\n  %v", group, got, want)
			}
		})
	}
}

// Thirteen buttons is the practical ceiling for muscle memory, and the PLAY tab
// is the one a coach uses without looking. Spec section 7 is explicit that the
// remedy for a busy tab is removing buttons, not adding a fourth tab, so a
// fourteenth kind here should fail a test before it reaches a match.
func TestPlayTabStaysAtItsButtonCeiling(t *testing.T) {
	if got, want := len(kindsInGroup(GroupPlay)), 13; got != want {
		t.Errorf("PLAY holds %d kinds, want %d", got, want)
	}
}

func TestCatalogueHasNoDuplicatesOrEmptyLabels(t *testing.T) {
	seen := map[EventKind]bool{}
	for _, entry := range Catalogue() {
		if entry.Kind == "" {
			t.Errorf("catalogue holds an entry with no kind: %+v", entry)
			continue
		}
		if seen[entry.Kind] {
			t.Errorf("catalogue holds %q twice", entry.Kind)
		}
		seen[entry.Kind] = true

		if entry.Label == "" {
			t.Errorf("%q has no label", entry.Kind)
		}
		if !slices.Contains(AllEventGroups(), entry.Group) {
			t.Errorf("%q is in unknown group %q", entry.Kind, entry.Group)
		}
	}
}

func TestAllEventKindsMatchesTheCatalogue(t *testing.T) {
	kinds := AllEventKinds()
	entries := Catalogue()
	if len(kinds) != len(entries) {
		t.Fatalf("AllEventKinds() has %d kinds, Catalogue() has %d entries", len(kinds), len(entries))
	}

	for _, entry := range entries {
		if !slices.Contains(kinds, entry.Kind) {
			t.Errorf("AllEventKinds() is missing %q", entry.Kind)
		}
	}
}

func TestLookupKind(t *testing.T) {
	for _, kind := range AllEventKinds() {
		entry, ok := LookupKind(kind)
		if !ok {
			t.Errorf("LookupKind(%q) ok = false, want true", kind)
			continue
		}
		if entry.Kind != kind {
			t.Errorf("LookupKind(%q) returned %q", kind, entry.Kind)
		}
	}

	for _, bad := range []EventKind{"", "tackle_almost", EventKind(GroupScore)} {
		if _, ok := LookupKind(bad); ok {
			t.Errorf("LookupKind(%q) ok = true, want false", bad)
		}
	}
}

// Points are what make a scoreboard possible; spec section 6 lists only `try`,
// which cannot produce one on its own. See the plan's "Decisions taken here".
func TestCataloguePoints(t *testing.T) {
	want := map[EventKind]int{
		KindTry:                  5,
		KindConversionMade:       2,
		KindPenaltyGoal:          3,
		KindDropGoal:             3,
		KindOppositionTry:        5,
		KindOppositionConversion: 2,
		KindOppositionPenalty:    3,
		KindOppositionDrop:       3,
	}

	for _, entry := range Catalogue() {
		if got := entry.Points; got != want[entry.Kind] {
			t.Errorf("%q is worth %d points, want %d", entry.Kind, got, want[entry.Kind])
		}
	}
}

// Our scores belong to whoever scored them; the opposition's are a scoreline and
// nothing more, because this application never holds their squad.
func TestScoringAttribution(t *testing.T) {
	for _, entry := range Catalogue() {
		if entry.Group != GroupScore {
			continue
		}
		opposition := entry.Kind == KindOppositionTry ||
			entry.Kind == KindOppositionConversion ||
			entry.Kind == KindOppositionPenalty ||
			entry.Kind == KindOppositionDrop

		if opposition && entry.Player {
			t.Errorf("%q is player-attributed, want team-level", entry.Kind)
		}
		if !opposition && !entry.Player {
			t.Errorf("%q is team-level, want player-attributed", entry.Kind)
		}
	}
}

// A control event is the clock and the log's own bookkeeping. None of it belongs
// to a player, and none of it appears on a tab.
func TestControlEventsAreTeamLevelAndUnscored(t *testing.T) {
	for _, kind := range kindsInGroup(GroupControl) {
		entry, _ := LookupKind(kind)
		if entry.Player {
			t.Errorf("%q is player-attributed, want team-level", kind)
		}
		if entry.Points != 0 {
			t.Errorf("%q is worth %d points, want 0", kind, entry.Points)
		}
	}
}

func TestDisciplineIsAlwaysPlayerAttributed(t *testing.T) {
	for _, kind := range kindsInGroup(GroupDiscipline) {
		entry, _ := LookupKind(kind)
		if !entry.Player {
			t.Errorf("%q is team-level, want player-attributed: a penalty is always someone's", kind)
		}
	}
}
