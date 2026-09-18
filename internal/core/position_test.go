package core

import "testing"

func TestPositionForJersey(t *testing.T) {
	cases := map[int]Position{
		1: Loosehead, 2: Hooker, 3: Tighthead,
		4: Lock, 5: Lock,
		6: Blindside, 7: Openside, 8: NumberEight,
		9: ScrumHalf, 10: FlyHalf,
		11: Wing, 14: Wing,
		12: InsideCentre, 13: OutsideCentre,
		15: Fullback,
	}

	for jersey, want := range cases {
		got, err := PositionForJersey(jersey)
		if err != nil {
			t.Errorf("PositionForJersey(%d) error = %v", jersey, err)
			continue
		}
		if got != want {
			t.Errorf("PositionForJersey(%d) = %q, want %q", jersey, got, want)
		}
	}
}

func TestPositionForJerseyRejectsOutOfRange(t *testing.T) {
	for _, jersey := range []int{0, -1, 16, 23, 99} {
		if _, err := PositionForJersey(jersey); err == nil {
			t.Errorf("PositionForJersey(%d) error = nil, want an error", jersey)
		}
	}
}

func TestPositionGroup(t *testing.T) {
	cases := map[Position]PositionGroup{
		Loosehead:     FrontRow,
		Hooker:        FrontRow,
		Tighthead:     FrontRow,
		Lock:          SecondRow,
		Blindside:     BackRow,
		Openside:      BackRow,
		NumberEight:   BackRow,
		ScrumHalf:     HalfBacks,
		FlyHalf:       HalfBacks,
		InsideCentre:  Centres,
		OutsideCentre: Centres,
		Wing:          BackThree,
		Fullback:      BackThree,
	}

	for position, want := range cases {
		if got := position.Group(); got != want {
			t.Errorf("Position(%q).Group() = %q, want %q", position, got, want)
		}
	}
}

func TestEveryPositionIsCoveredByAGroup(t *testing.T) {
	for _, p := range AllPositions() {
		if !p.Valid() {
			t.Errorf("AllPositions() returned invalid position %q", p)
		}
		if p.Group() == "" {
			t.Errorf("Position(%q) has no group", p)
		}
	}
}

func TestUnknownPositionHasNoGroup(t *testing.T) {
	if got := Position("prop").Group(); got != "" {
		t.Errorf(`Position("prop").Group() = %q, want ""`, got)
	}
	if Position("prop").Valid() {
		t.Error(`Position("prop").Valid() = true, want false`)
	}
}

func TestPositionGroupForJersey(t *testing.T) {
	cases := map[int]PositionGroup{
		1: FrontRow, 4: SecondRow, 7: BackRow,
		9: HalfBacks, 12: Centres, 13: Centres,
		11: BackThree, 14: BackThree, 15: BackThree,
	}

	for jersey, want := range cases {
		got, err := PositionGroupForJersey(jersey)
		if err != nil {
			t.Errorf("PositionGroupForJersey(%d) error = %v", jersey, err)
			continue
		}
		if got != want {
			t.Errorf("PositionGroupForJersey(%d) = %q, want %q", jersey, got, want)
		}
	}
}
