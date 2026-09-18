package core

import "fmt"

// Position is an individual playing position. Thirteen positions cover the
// fifteen starting jerseys: both locks wear 4 and 5, and both wings wear 11
// and 14.
type Position string

const (
	Loosehead     Position = "loosehead"      // 1
	Hooker        Position = "hooker"         // 2
	Tighthead     Position = "tighthead"      // 3
	Lock          Position = "lock"           // 4, 5
	Blindside     Position = "blindside"      // 6
	Openside      Position = "openside"       // 7
	NumberEight   Position = "number_eight"   // 8
	ScrumHalf     Position = "scrum_half"     // 9
	FlyHalf       Position = "fly_half"       // 10
	Wing          Position = "wing"           // 11, 14
	InsideCentre  Position = "inside_centre"  // 12
	OutsideCentre Position = "outside_centre" // 13
	Fullback      Position = "fullback"       // 15
)

// PositionGroup is the unit within which players are compared. Comparing a
// prop's carry count against a winger's is noise, so rankings never cross a
// group boundary.
type PositionGroup string

const (
	FrontRow  PositionGroup = "front_row"
	SecondRow PositionGroup = "second_row"
	BackRow   PositionGroup = "back_row"
	HalfBacks PositionGroup = "half_backs"
	Centres   PositionGroup = "centres"
	BackThree PositionGroup = "back_three"
)

// positionGroups is the single source of truth for which positions exist and
// which group each belongs to.
var positionGroups = map[Position]PositionGroup{
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

// jerseyPositions maps a starting jersey to the position that wears it.
var jerseyPositions = map[int]Position{
	1: Loosehead, 2: Hooker, 3: Tighthead,
	4: Lock, 5: Lock,
	6: Blindside, 7: Openside, 8: NumberEight,
	9: ScrumHalf, 10: FlyHalf,
	11: Wing,
	12: InsideCentre, 13: OutsideCentre,
	14: Wing,
	15: Fullback,
}

// AllPositions returns every position in jersey order, for building pickers.
func AllPositions() []Position {
	return []Position{
		Loosehead, Hooker, Tighthead,
		Lock,
		Blindside, Openside, NumberEight,
		ScrumHalf, FlyHalf,
		Wing, InsideCentre, OutsideCentre, Fullback,
	}
}

// AllPositionGroups returns every comparison group, forwards-to-backs.
func AllPositionGroups() []PositionGroup {
	return []PositionGroup{FrontRow, SecondRow, BackRow, HalfBacks, Centres, BackThree}
}

// Group returns the position's comparison group, or "" if the position is unknown.
func (p Position) Group() PositionGroup {
	return positionGroups[p]
}

// Valid reports whether the position is one of the known constants.
func (p Position) Valid() bool {
	_, ok := positionGroups[p]
	return ok
}

// PositionForJersey returns the position worn by a starting jersey (1-15).
func PositionForJersey(jersey int) (Position, error) {
	position, ok := jerseyPositions[jersey]
	if !ok {
		return "", fmt.Errorf("core: jersey %d is not a starting number (1-15)", jersey)
	}
	return position, nil
}

// PositionGroupForJersey returns the comparison group for a starting jersey.
func PositionGroupForJersey(jersey int) (PositionGroup, error) {
	position, err := PositionForJersey(jersey)
	if err != nil {
		return "", err
	}
	return position.Group(), nil
}
