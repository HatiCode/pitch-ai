package core

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"
)

func playerID(jersey int) string {
	return "p" + strconv.Itoa(jersey)
}

func mins(m int) int {
	return m * 60 * 1000
}

// taggedMatch is a fixture with a full selection: fifteen starters and eight
// replacements, with predictable player IDs.
func taggedMatch() Match {
	var lineup Lineup
	for jersey := firstStarterJersey; jersey <= lastStarterJersey; jersey++ {
		lineup.Starters = append(lineup.Starters, LineupSlot{Jersey: jersey, PlayerID: playerID(jersey)})
	}
	for jersey := firstBenchJersey; jersey <= lastBenchJersey; jersey++ {
		lineup.Bench = append(lineup.Bench, LineupSlot{Jersey: jersey, PlayerID: playerID(jersey)})
	}

	return Match{
		ID:        "match-1",
		ClubID:    "club-1",
		TeamID:    "team-1",
		SeasonID:  "2026-27",
		Opponent:  "Lansdowne",
		KickoffAt: time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC),
		Venue:     VenueHome,
		Status:    MatchScheduled,
		Lineup:    lineup,
	}
}

// logOf numbers events in declaration order, so a test can reason about how ties
// on the same clock value are broken.
func logOf(events ...Event) []Event {
	for i := range events {
		events[i].ID = fmt.Sprintf("e%03d", i+1)
		if events[i].DeviceID == "" {
			events[i].DeviceID = "device-1"
		}
		if events[i].Period == 0 {
			events[i].Period = 1
		}
	}
	return events
}

func control(kind EventKind, clockMs int) Event {
	return Event{Kind: kind, ClockMs: clockMs}
}

func tagged(kind EventKind, clockMs int, player string) Event {
	return Event{Kind: kind, ClockMs: clockMs, PlayerID: player}
}

func teamEvent(kind EventKind, clockMs int) Event {
	return Event{Kind: kind, ClockMs: clockMs}
}

func zoneChange(clockMs int, zone Zone, possession Possession) Event {
	return Event{
		Kind:    KindZoneChanged,
		ClockMs: clockMs,
		Payload: map[string]string{
			PayloadZone:       string(zone),
			PayloadPossession: string(possession),
		},
	}
}

func substitution(clockMs int, on, off string) Event {
	return Event{
		Kind:    KindSub,
		ClockMs: clockMs,
		Payload: map[string]string{PayloadOnPlayer: on, PayloadOffPlayer: off},
	}
}

func voiding(clockMs int, targetID string) Event {
	return Event{Kind: KindVoid, ClockMs: clockMs, VoidsID: targetID}
}

func inPeriod(period int, e Event) Event {
	e.Period = period
	return e
}

func TestFoldEmptyLog(t *testing.T) {
	state := Fold(taggedMatch(), nil)

	if state.Status != MatchScheduled {
		t.Errorf("Status = %q, want %q", state.Status, MatchScheduled)
	}
	if state.ClockMs != 0 || state.Running || state.Period != 0 {
		t.Errorf("clock = %d/%v/period %d, want 0/false/0", state.ClockMs, state.Running, state.Period)
	}
	if state.Score != (Score{}) {
		t.Errorf("Score = %+v, want 0-0", state.Score)
	}
	if state.Zone != ZoneOurHalf || state.Possession != PossessionUs {
		t.Errorf("toggles = %q/%q, want our_half/us", state.Zone, state.Possession)
	}
	if len(state.Players) != 23 {
		t.Fatalf("Players has %d entries, want 23", len(state.Players))
	}
	if len(state.Slices) != 0 {
		t.Errorf("Slices has %d entries, want none before kick-off", len(state.Slices))
	}

	// Nil maps and nil slices serialise differently from empty ones, and the
	// TypeScript fold has to match this JSON exactly.
	if state.TerritoryMs == nil || state.PossessionMs == nil || state.Counts == nil || state.Slices == nil || state.VoidedIDs == nil {
		t.Error("Fold returned a nil map or slice; every collection must be initialised")
	}

	hooker := state.Players[playerID(2)]
	if !hooker.OnPitch || hooker.MinutesMs != 0 {
		t.Errorf("starter = onPitch %v, %d ms, want true, 0", hooker.OnPitch, hooker.MinutesMs)
	}
	if hooker.Position != Hooker || hooker.Group != FrontRow {
		t.Errorf("jersey 2 = %q/%q, want hooker/front_row", hooker.Position, hooker.Group)
	}

	replacement := state.Players[playerID(18)]
	if replacement.OnPitch {
		t.Error("a replacement starts off the pitch")
	}
}

func TestFoldCountsBelongToTheTaggedPlayer(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTackleMade, mins(1), playerID(7)),
		tagged(KindTackleMade, mins(2), playerID(7)),
		tagged(KindTackleMissed, mins(3), playerID(7)),
		tagged(KindCarry, mins(4), playerID(12)),
		teamEvent(KindScrumWon, mins(5)),
	)

	state := Fold(taggedMatch(), events)

	flanker := state.Players[playerID(7)]
	if got := flanker.Counts[KindTackleMade]; got != 2 {
		t.Errorf("p7 tackles made = %d, want 2", got)
	}
	if got := flanker.Counts[KindTackleMissed]; got != 1 {
		t.Errorf("p7 tackles missed = %d, want 1", got)
	}
	if got := flanker.Counts[KindCarry]; got != 0 {
		t.Errorf("p7 carries = %d, want 0: the carry was p12's", got)
	}
	if got := state.Players[playerID(12)].Counts[KindCarry]; got != 1 {
		t.Errorf("p12 carries = %d, want 1", got)
	}

	// Team-level kinds have no player, so the match total is the only place a
	// scrum count can live.
	if got := state.Counts[KindScrumWon]; got != 1 {
		t.Errorf("match scrums won = %d, want 1", got)
	}
	if got := state.Counts[KindTackleMade]; got != 2 {
		t.Errorf("match tackles made = %d, want 2", got)
	}
}

func TestFoldScoreSumsBothSides(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTry, mins(10), playerID(11)),
		tagged(KindConversionMade, mins(11), playerID(10)),
		tagged(KindPenaltyGoal, mins(20), playerID(10)),
		tagged(KindConversionMissed, mins(25), playerID(10)),
		teamEvent(KindOppositionTry, mins(30)),
		teamEvent(KindOppositionConversion, mins(31)),
	)

	state := Fold(taggedMatch(), events)

	if want := (Score{Us: 10, Them: 7}); state.Score != want {
		t.Errorf("Score = %+v, want %+v", state.Score, want)
	}
	// A missed kick is worth nothing but still counts, because goal-kicking
	// percentage is the point of recording it.
	if got := state.Players[playerID(10)].Counts[KindConversionMissed]; got != 1 {
		t.Errorf("p10 conversions missed = %d, want 1", got)
	}
}

func TestFoldClockDerivesFromControlEvents(t *testing.T) {
	running := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindCarry, mins(10), playerID(12)),
	)
	state := Fold(taggedMatch(), running)
	if state.ClockMs != mins(10) || !state.Running {
		t.Errorf("after a carry at 10:00 clock = %d/%v, want %d/true", state.ClockMs, state.Running, mins(10))
	}
	if state.Period != 1 {
		t.Errorf("Period = %d, want 1", state.Period)
	}

	stopped := logOf(
		control(KindPeriodStarted, 0),
		control(KindClockPaused, mins(10)),
	)
	state = Fold(taggedMatch(), stopped)
	if state.ClockMs != mins(10) || state.Running {
		t.Errorf("after a pause at 10:00 clock = %d/%v, want %d/false", state.ClockMs, state.Running, mins(10))
	}
}

func TestFoldMinutesCountOnlyRunningClock(t *testing.T) {
	// The clock does not advance while it is stopped, so the resume carries the
	// same value as the pause.
	events := logOf(
		control(KindPeriodStarted, 0),
		control(KindClockPaused, mins(10)),
		control(KindClockResumed, mins(10)),
		control(KindPeriodEnded, mins(20)),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(1)].MinutesMs; got != mins(20) {
		t.Errorf("minutes = %d, want %d", got, mins(20))
	}
}

// A second device that has not yet pulled the pause goes on stamping a clock that
// advances. Its events must not resurrect time the referee stopped.
func TestFoldIgnoresSpansStampedDuringAStoppage(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		control(KindClockPaused, mins(10)),
		tagged(KindTackleMade, mins(15), playerID(7)),
		control(KindClockResumed, mins(15)),
		control(KindPeriodEnded, mins(25)),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(1)].MinutesMs; got != mins(20) {
		t.Errorf("minutes = %d, want %d: the five stopped minutes must not count", got, mins(20))
	}
	if got := state.Players[playerID(7)].Counts[KindTackleMade]; got != 1 {
		t.Errorf("p7 tackles = %d, want 1: the tackle still happened", got)
	}
}

func TestFoldSubstitutionSplitsMinutes(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		substitution(mins(40), playerID(18), playerID(7)),
		control(KindMatchEnded, mins(80)),
	)

	state := Fold(taggedMatch(), events)

	off := state.Players[playerID(7)]
	on := state.Players[playerID(18)]
	if off.MinutesMs != mins(40) {
		t.Errorf("p7 minutes = %d, want %d", off.MinutesMs, mins(40))
	}
	if on.MinutesMs != mins(40) {
		t.Errorf("p18 minutes = %d, want %d", on.MinutesMs, mins(40))
	}
	if off.OnPitch || !on.OnPitch {
		t.Errorf("onPitch: p7 %v, p18 %v; want false, true", off.OnPitch, on.OnPitch)
	}
	if got := state.Players[playerID(1)].MinutesMs; got != mins(80) {
		t.Errorf("an unchanged starter has %d ms, want %d", got, mins(80))
	}
	// A replacement's position is whoever they came on for: the lineup only knows
	// their bench number.
	if on.Position != Openside || on.Group != BackRow {
		t.Errorf("p18 = %q/%q, want openside/back_row inherited from p7", on.Position, on.Group)
	}
}

// Ten minutes of sin-bin is ten minutes of running clock. Counting wall clock
// would let the bin expire during a stoppage and hand back minutes never played.
func TestFoldYellowCardCostsTenMinutesOfRunningClock(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindYellowCard, mins(10), playerID(7)),
		control(KindClockPaused, mins(15)),
		control(KindClockResumed, mins(15)),
		control(KindMatchEnded, mins(75)),
	)

	state := Fold(taggedMatch(), events)

	// 75 minutes of running clock; p7 misses ten of them and returns afterwards.
	if got := state.Players[playerID(1)].MinutesMs; got != mins(75) {
		t.Fatalf("an uncarded starter has %d ms, want %d", got, mins(75))
	}
	carded := state.Players[playerID(7)]
	if carded.MinutesMs != mins(65) {
		t.Errorf("p7 minutes = %d, want %d", carded.MinutesMs, mins(65))
	}
	if !carded.OnPitch {
		t.Error("p7 should be back on the pitch once the bin expires")
	}
	if got := carded.Counts[KindYellowCard]; got != 1 {
		t.Errorf("p7 yellow cards = %d, want 1", got)
	}
}

func TestFoldYellowCardExpiresAcrossAStoppage(t *testing.T) {
	// Carded at 10:00, then the clock is stopped for the whole of what would have
	// been the bin. The player must still owe ten running minutes on the restart.
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindYellowCard, mins(10), playerID(7)),
		control(KindClockPaused, mins(10)),
		control(KindClockResumed, mins(10)),
		control(KindMatchEnded, mins(30)),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(7)].MinutesMs; got != mins(20) {
		t.Errorf("p7 minutes = %d, want %d (10 before the card, 10 after the bin)", got, mins(20))
	}
}

func TestFoldRedCardRemovesThePlayerPermanently(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindRedCard, mins(20), playerID(7)),
		control(KindMatchEnded, mins(80)),
	)

	state := Fold(taggedMatch(), events)

	sentOff := state.Players[playerID(7)]
	if sentOff.MinutesMs != mins(20) {
		t.Errorf("p7 minutes = %d, want %d", sentOff.MinutesMs, mins(20))
	}
	if sentOff.OnPitch {
		t.Error("a red-carded player never returns")
	}
}

func TestFoldVoidRemovesItsTarget(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),          // e001
		tagged(KindTackleMade, 1, playerID(7)), // e002
		tagged(KindTackleMade, 2, playerID(7)), // e003
		tagged(KindTackleMade, 3, playerID(7)), // e004
		voiding(4, "e003"),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(7)].Counts[KindTackleMade]; got != 2 {
		t.Errorf("p7 tackles = %d, want 2 after one was voided", got)
	}
	if got := state.Counts[KindTackleMade]; got != 2 {
		t.Errorf("match tackles = %d, want 2", got)
	}
	if !slices.Contains(state.VoidedIDs, "e003") {
		t.Errorf("VoidedIDs = %v, want it to hold e003", state.VoidedIDs)
	}
	// The void is bookkeeping, not an occurrence, so it is never counted itself.
	if got := state.Counts[KindVoid]; got != 0 {
		t.Errorf("match voids counted = %d, want 0", got)
	}
}

func TestFoldVoidUndoesAScore(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),           // e001
		tagged(KindTry, mins(10), playerID(11)), // e002
		voiding(mins(11), "e002"),
	)

	state := Fold(taggedMatch(), events)

	if state.Score != (Score{}) {
		t.Errorf("Score = %+v, want 0-0 once the try is voided", state.Score)
	}
}

func TestFoldVoidUndoesASubstitution(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),                     // e001
		substitution(mins(40), playerID(18), playerID(7)), // e002
		voiding(mins(41), "e002"),
		control(KindMatchEnded, mins(80)),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(7)].MinutesMs; got != mins(80) {
		t.Errorf("p7 minutes = %d, want %d: the substitution was voided", got, mins(80))
	}
	if got := state.Players[playerID(18)].MinutesMs; got != 0 {
		t.Errorf("p18 minutes = %d, want 0", got)
	}
}

// Voiding a void is not a resurrection. The tagging screen only ever voids live
// events, and a cancel-the-cancel rule would make the log's meaning depend on the
// order it was read in.
func TestFoldVoidOfAVoidIsNotAResurrection(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),          // e001
		tagged(KindTackleMade, 1, playerID(7)), // e002
		voiding(2, "e002"),                     // e003
		voiding(3, "e003"),
	)

	state := Fold(taggedMatch(), events)

	if got := state.Players[playerID(7)].Counts[KindTackleMade]; got != 0 {
		t.Errorf("p7 tackles = %d, want 0: the original stays cancelled", got)
	}
}

func TestFoldTerritoryAndPossessionAccumulateBetweenToggles(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		zoneChange(mins(10), ZoneTheir22, PossessionUs),
		zoneChange(mins(25), ZoneOurHalf, PossessionThem),
		control(KindMatchEnded, mins(30)),
	)

	state := Fold(taggedMatch(), events)

	// our_half/us for the first ten minutes, their_22/us for fifteen, then
	// our_half/them for five.
	if got := state.TerritoryMs[ZoneOurHalf]; got != mins(15) {
		t.Errorf("our_half = %d, want %d", got, mins(15))
	}
	if got := state.TerritoryMs[ZoneTheir22]; got != mins(15) {
		t.Errorf("their_22 = %d, want %d", got, mins(15))
	}
	if got := state.PossessionMs[PossessionUs]; got != mins(25) {
		t.Errorf("possession us = %d, want %d", got, mins(25))
	}
	if got := state.PossessionMs[PossessionThem]; got != mins(5) {
		t.Errorf("possession them = %d, want %d", got, mins(5))
	}
	if state.Zone != ZoneOurHalf || state.Possession != PossessionThem {
		t.Errorf("final toggles = %q/%q, want our_half/them", state.Zone, state.Possession)
	}
}

// A stoppage must not dilute a territory figure: the percentage is only worth
// showing a coach if its denominator is time the ball was actually in play.
func TestFoldStoppageDoesNotDiluteTerritory(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		zoneChange(mins(5), ZoneOur22, PossessionThem),
		control(KindClockPaused, mins(10)),
		control(KindClockResumed, mins(10)),
		control(KindMatchEnded, mins(15)),
	)

	state := Fold(taggedMatch(), events)

	if got := state.TerritoryMs[ZoneOur22]; got != mins(10) {
		t.Errorf("our_22 = %d, want %d", got, mins(10))
	}
	total := 0
	for _, ms := range state.TerritoryMs {
		total += ms
	}
	if total != mins(15) {
		t.Errorf("territory total = %d, want %d: only running clock accumulates", total, mins(15))
	}
}

// Appends from two devices interleave into one log. Folding it must not depend on
// the order they happened to arrive in, because that is what lets two coaches tag
// the same match with no merge algorithm.
func TestFoldIsOrderIndependent(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTackleMade, mins(3), playerID(7)),
		zoneChange(mins(5), ZoneTheirHalf, PossessionUs),
		tagged(KindYellowCard, mins(10), playerID(4)),
		substitution(mins(40), playerID(18), playerID(7)),
		tagged(KindTry, mins(50), playerID(11)),
		tagged(KindConversionMade, mins(51), playerID(10)),
		teamEvent(KindOppositionTry, mins(60)),
		zoneChange(mins(70), ZoneOur22, PossessionThem),
		control(KindMatchEnded, mins(80)),
	)
	events[1].DeviceID = "device-2"
	events[5].DeviceID = "device-2"

	want := Fold(taggedMatch(), events)

	shuffled := slices.Clone(events)
	shuffler := rand.New(rand.NewPCG(7, 11))
	shuffler.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	if got := Fold(taggedMatch(), shuffled); !reflect.DeepEqual(got, want) {
		t.Errorf("folding a shuffled log gave a different state:\n got %+v\nwant %+v", got, want)
	}
}

func TestFoldSlicesSplitSpansAtTheBoundary(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		zoneChange(mins(19), ZoneTheir22, PossessionUs),
		zoneChange(mins(22), ZoneOurHalf, PossessionUs),
		control(KindMatchEnded, mins(25)),
	)

	state := Fold(taggedMatch(), events)

	if len(state.Slices) != 2 {
		t.Fatalf("Slices has %d entries, want 2 for a log reaching 25:00", len(state.Slices))
	}
	if got, want := state.Slices[0].FromMs, 0; got != want {
		t.Errorf("slice 0 starts at %d, want %d", got, want)
	}
	if got, want := state.Slices[1].FromMs, SliceDurationMs; got != want {
		t.Errorf("slice 1 starts at %d, want %d", got, want)
	}
	if got := state.Slices[0].TerritoryMs[ZoneTheir22]; got != mins(1) {
		t.Errorf("their_22 in slice 0 = %d, want %d", got, mins(1))
	}
	if got := state.Slices[1].TerritoryMs[ZoneTheir22]; got != mins(2) {
		t.Errorf("their_22 in slice 1 = %d, want %d", got, mins(2))
	}
}

func TestFoldSlicesCarryScoresAndCounts(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTry, mins(10), playerID(11)),
		inPeriod(2, tagged(KindTry, mins(50), playerID(14))),
		inPeriod(2, teamEvent(KindOppositionPenalty, mins(55))),
	)

	state := Fold(taggedMatch(), events)

	if len(state.Slices) != 3 {
		t.Fatalf("Slices has %d entries, want 3", len(state.Slices))
	}
	if want := (Score{Us: 5}); state.Slices[0].Score != want {
		t.Errorf("slice 0 score = %+v, want %+v", state.Slices[0].Score, want)
	}
	if state.Slices[1].Score != (Score{}) {
		t.Errorf("slice 1 score = %+v, want 0-0", state.Slices[1].Score)
	}
	if want := (Score{Us: 5, Them: 3}); state.Slices[2].Score != want {
		t.Errorf("slice 2 score = %+v, want %+v", state.Slices[2].Score, want)
	}
	if got := state.Slices[2].Counts[KindTry]; got != 1 {
		t.Errorf("tries in slice 2 = %d, want 1", got)
	}
}

func TestFoldStatusFollowsTheLog(t *testing.T) {
	cases := []struct {
		name   string
		events []Event
		want   MatchStatus
	}{
		{"no events", nil, MatchScheduled},
		{
			"a tagged event",
			logOf(control(KindPeriodStarted, 0), tagged(KindCarry, mins(1), playerID(12))),
			MatchInProgress,
		},
		{
			"the match ended",
			logOf(control(KindPeriodStarted, 0), control(KindMatchEnded, mins(80))),
			MatchCompleted,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fold(taggedMatch(), c.events).Status; got != c.want {
				t.Errorf("Status = %q, want %q", got, c.want)
			}
		})
	}
}

// A player removed from the lineup after being tagged must not panic the fold or
// invent a stats row nobody selected.
func TestFoldSkipsPlayersOutsideTheLineup(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTackleMade, mins(1), "p99"),
		tagged(KindTackleMade, mins(2), playerID(7)),
	)

	state := Fold(taggedMatch(), events)

	if _, ok := state.Players["p99"]; ok {
		t.Error("Fold invented a stats row for an unselected player")
	}
	if got := state.Players[playerID(7)].Counts[KindTackleMade]; got != 1 {
		t.Errorf("p7 tackles = %d, want 1", got)
	}
	if got := state.Counts[KindTackleMade]; got != 1 {
		t.Errorf("match tackles = %d, want 1: an unattributable event is dropped", got)
	}
}

func TestFoldDoesNotMutateItsInput(t *testing.T) {
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTackleMade, mins(2), playerID(7)),
		voiding(mins(3), "e002"),
		control(KindMatchEnded, mins(80)),
	)
	before := slices.Clone(events)

	Fold(taggedMatch(), events)

	if !reflect.DeepEqual(events, before) {
		t.Error("Fold reordered or rewrote the events it was given")
	}
}

func TestPlayerMatchStatsDenormalisesForSeasonQueries(t *testing.T) {
	match := taggedMatch()
	events := logOf(
		control(KindPeriodStarted, 0),
		tagged(KindTackleMade, mins(5), playerID(7)),
		control(KindMatchEnded, mins(80)),
	)

	stats := Fold(match, events).PlayerMatchStats(match)

	if len(stats) != 23 {
		t.Fatalf("PlayerMatchStats has %d rows, want 23", len(stats))
	}
	if !slices.IsSortedFunc(stats, func(a, b PlayerMatchStats) int { return a.Jersey - b.Jersey }) {
		t.Error("rows are not in jersey order, so a batch write would not be stable")
	}

	first := stats[0]
	if first.Jersey != 1 || first.PlayerID != playerID(1) {
		t.Errorf("first row = jersey %d, %q; want jersey 1, p1", first.Jersey, first.PlayerID)
	}
	if first.MatchID != match.ID || first.TeamID != match.TeamID || first.SeasonID != match.SeasonID {
		t.Errorf("row = %+v, want match, team and season denormalised", first)
	}
	if !first.MatchDate.Equal(match.KickoffAt) {
		t.Errorf("MatchDate = %v, want %v", first.MatchDate, match.KickoffAt)
	}
	if first.MinutesMs != mins(80) {
		t.Errorf("MinutesMs = %d, want %d", first.MinutesMs, mins(80))
	}

	flanker := stats[6]
	if flanker.PlayerID != playerID(7) {
		t.Fatalf("stats[6] is %q, want p7", flanker.PlayerID)
	}
	if got := flanker.Counts[string(KindTackleMade)]; got != 1 {
		t.Errorf("p7 tackles = %d, want 1", got)
	}
}
