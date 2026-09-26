package core

import (
	"slices"
	"strings"
	"time"
)

// SliceDurationMs is the width of a time-slice bucket. ClockMs is continuous from
// kick-off, so bucketing every metric into 20-minute blocks is one grouping here
// and needs no extra data, no extra writes and no schema change.
const SliceDurationMs = 20 * 60 * 1000

// sinBinMs is a yellow card's cost, spent in running clock rather than wall clock.
// Counting wall clock would let the bin expire during a stoppage and hand back
// minutes the player never played.
const sinBinMs = 10 * 60 * 1000

type Score struct {
	Us   int `json:"us"`
	Them int `json:"them"`
}

// PlayerStats is one player's contribution to one match. Counts holds only
// non-zero entries, so the Go and TypeScript folds serialise identically.
type PlayerStats struct {
	PlayerID  string            `json:"playerId"`
	Jersey    int               `json:"jersey"`
	Position  Position          `json:"position"`
	Group     PositionGroup     `json:"group"`
	OnPitch   bool              `json:"onPitch"`
	MinutesMs int               `json:"minutesMs"`
	Counts    map[EventKind]int `json:"counts"`
}

// Slice is one 20-minute block. Per-player slicing is deliberately absent: it is a
// season-view concern and would multiply this structure by 23 for no reader.
type Slice struct {
	FromMs       int                `json:"fromMs"`
	ToMs         int                `json:"toMs"`
	Score        Score              `json:"score"`
	TerritoryMs  map[Zone]int       `json:"territoryMs"`
	PossessionMs map[Possession]int `json:"possessionMs"`
	Counts       map[EventKind]int  `json:"counts"`
}

// MatchState is the whole state of a match. It is derived, never stored as truth:
// void an event, re-fold, and every number below is correct by construction.
type MatchState struct {
	Status       MatchStatus            `json:"status"`
	Period       int                    `json:"period"`
	ClockMs      int                    `json:"clockMs"`
	Running      bool                   `json:"running"`
	Score        Score                  `json:"score"`
	Zone         Zone                   `json:"zone"`
	Possession   Possession             `json:"possession"`
	TerritoryMs  map[Zone]int           `json:"territoryMs"`
	PossessionMs map[Possession]int     `json:"possessionMs"`
	Counts       map[EventKind]int      `json:"counts"`
	Players      map[string]PlayerStats `json:"players"`
	Slices       []Slice                `json:"slices"`
	VoidedIDs    []string               `json:"voidedIds"`
}

// PlayerMatchStats is the derived per-player document. It denormalises team,
// season, player and date so a season trend or a squad comparison is a
// collection-group query rather than a rollup table that can go stale.
type PlayerMatchStats struct {
	PlayerID  string         `json:"playerId" firestore:"playerId"`
	MatchID   string         `json:"matchId" firestore:"matchId"`
	TeamID    string         `json:"teamId" firestore:"teamId"`
	SeasonID  string         `json:"seasonId" firestore:"seasonId"`
	MatchDate time.Time      `json:"matchDate" firestore:"matchDate"`
	Jersey    int            `json:"jersey" firestore:"jersey"`
	Position  Position       `json:"position" firestore:"position"`
	Group     PositionGroup  `json:"group" firestore:"group"`
	MinutesMs int            `json:"minutesMs" firestore:"minutesMs"`
	Counts    map[string]int `json:"counts" firestore:"counts"`
}

// Fold derives the whole state of a match from its event log. It is pure and
// total: no clock, no I/O, no error. Feed it the same events in any order and it
// returns the same state, which is what makes two coaches tagging one match and a
// correction after the fact both safe.
//
// Time accumulates up to the clock of the last event in the log. For a finished
// match that is exact; during live tagging per-player minutes therefore lag by one
// tap, which no coach is reading mid-match.
func Fold(m Match, events []Event) MatchState {
	voided := voidedIDs(events)
	cancelled := make(map[string]bool, len(voided))
	for _, id := range voided {
		cancelled[id] = true
	}

	state := newState(m)
	state.VoidedIDs = voided

	f := newFolder(&state, m)
	for _, e := range sortedLive(events, cancelled) {
		f.advanceTo(e.ClockMs)
		f.apply(e)
	}
	f.finish()

	return state
}

// PlayerMatchStats projects the folded state into the derived documents that
// season queries read, in jersey order so a batch write is stable.
func (s MatchState) PlayerMatchStats(m Match) []PlayerMatchStats {
	out := make([]PlayerMatchStats, 0, len(s.Players))
	for _, p := range s.Players {
		counts := make(map[string]int, len(p.Counts))
		for kind, n := range p.Counts {
			counts[string(kind)] = n
		}

		out = append(out, PlayerMatchStats{
			PlayerID:  p.PlayerID,
			MatchID:   m.ID,
			TeamID:    m.TeamID,
			SeasonID:  m.SeasonID,
			MatchDate: m.KickoffAt,
			Jersey:    p.Jersey,
			Position:  p.Position,
			Group:     p.Group,
			MinutesMs: p.MinutesMs,
			Counts:    counts,
		})
	}

	slices.SortFunc(out, func(a, b PlayerMatchStats) int { return a.Jersey - b.Jersey })
	return out
}

// voidedIDs collects every cancellation in the log. A void that is itself voided
// still counts: the tagging screen only ever voids live events, and a
// cancel-the-cancel rule would make the log's meaning depend on read order.
func voidedIDs(events []Event) []string {
	ids := []string{}
	for _, e := range events {
		if e.Kind == KindVoid && e.VoidsID != "" {
			ids = append(ids, e.VoidsID)
		}
	}

	slices.Sort(ids)
	return slices.Compact(ids)
}

// sortedLive drops cancelled events and the voids themselves, then orders what is
// left by clock with the event ID breaking ties. Ids are UUIDv7 and so ordered by
// creation time, which gives two devices tagging the same instant one stable
// order. The input slice is never touched.
func sortedLive(events []Event, cancelled map[string]bool) []Event {
	live := make([]Event, 0, len(events))
	for _, e := range events {
		if e.Kind == KindVoid || cancelled[e.ID] {
			continue
		}
		live = append(live, e)
	}

	slices.SortFunc(live, func(a, b Event) int {
		if a.ClockMs != b.ClockMs {
			return a.ClockMs - b.ClockMs
		}
		return strings.Compare(a.ID, b.ID)
	})
	return live
}

// newState seeds the state from the selection. Every collection is initialised,
// because a nil map and an empty one serialise differently and the TypeScript fold
// has to match this JSON exactly.
func newState(m Match) MatchState {
	state := MatchState{
		Status:       MatchScheduled,
		Zone:         ZoneOurHalf,
		Possession:   PossessionUs,
		TerritoryMs:  map[Zone]int{},
		PossessionMs: map[Possession]int{},
		Counts:       map[EventKind]int{},
		Players:      map[string]PlayerStats{},
		Slices:       []Slice{},
		VoidedIDs:    []string{},
	}

	for _, slot := range m.Lineup.Starters {
		// A starting jersey names a position; a bench number does not, so a
		// replacement's position is filled in when they come on.
		position, _ := PositionForJersey(slot.Jersey)
		state.Players[slot.PlayerID] = PlayerStats{
			PlayerID: slot.PlayerID,
			Jersey:   slot.Jersey,
			Position: position,
			Group:    position.Group(),
			Counts:   map[EventKind]int{},
		}
	}
	for _, slot := range m.Lineup.Bench {
		state.Players[slot.PlayerID] = PlayerStats{
			PlayerID: slot.PlayerID,
			Jersey:   slot.Jersey,
			Counts:   map[EventKind]int{},
		}
	}
	return state
}

// slice returns the 20-minute bucket at index, growing the list so it stays
// contiguous from kick-off: a report renders the blocks in order and cannot have a
// hole in the middle.
func (s *MatchState) slice(index int) *Slice {
	for len(s.Slices) <= index {
		start := len(s.Slices) * SliceDurationMs
		s.Slices = append(s.Slices, Slice{
			FromMs:       start,
			ToMs:         start + SliceDurationMs,
			TerritoryMs:  map[Zone]int{},
			PossessionMs: map[Possession]int{},
			Counts:       map[EventKind]int{},
		})
	}
	return &s.Slices[index]
}

// folder carries the working state of one walk over the log. These fields are how
// the state is derived rather than part of it, which is why they do not live on
// MatchState.
type folder struct {
	state      *MatchState
	running    bool
	lastClock  int
	zone       Zone
	possession Possession
	onPitch    map[string]bool
	binBudget  map[string]int
	sentOff    map[string]bool
}

func newFolder(state *MatchState, m Match) *folder {
	onPitch := make(map[string]bool, len(m.Lineup.Starters))
	for _, slot := range m.Lineup.Starters {
		onPitch[slot.PlayerID] = true
	}

	return &folder{
		state:      state,
		zone:       state.Zone,
		possession: state.Possession,
		onPitch:    onPitch,
		binBudget:  map[string]int{},
		sentOff:    map[string]bool{},
	}
}

// advanceTo attributes the span between the last event and this one. A stopped
// clock contributes nothing — that is the whole of the pause behaviour, and it is
// why a stoppage cannot leak into a single denominator.
//
// Accumulating span by span, rather than opening and closing an interval per
// player, is what keeps stoppages, substitutions and sin-bins from needing three
// separate rules.
func (f *folder) advanceTo(clockMs int) {
	from, to := f.lastClock, clockMs
	if to > from {
		f.lastClock = to
	}
	if !f.running || to <= from {
		return
	}

	span := to - from
	f.state.TerritoryMs[f.zone] += span
	f.state.PossessionMs[f.possession] += span
	addSpan(from, to, func(index, ms int) {
		bucket := f.state.slice(index)
		bucket.TerritoryMs[f.zone] += ms
		bucket.PossessionMs[f.possession] += ms
	})

	for playerID := range f.onPitch {
		f.addMinutes(playerID, span)
	}
	f.expireBins(span)
}

// expireBins spends running clock against every sin-bin. A bin that runs out part
// way through a span puts its player back on for the remainder, which is why the
// budget is milliseconds owed rather than a return time.
func (f *folder) expireBins(span int) {
	for playerID, remaining := range f.binBudget {
		if remaining > span {
			f.binBudget[playerID] = remaining - span
			continue
		}

		delete(f.binBudget, playerID)
		f.onPitch[playerID] = true
		f.addMinutes(playerID, span-remaining)
	}
}

func (f *folder) apply(e Event) {
	entry, ok := LookupKind(e.Kind)
	if !ok {
		return
	}
	if f.state.Status == MatchScheduled {
		f.state.Status = MatchInProgress
	}

	// Control events drive the clock and the toggles. None of them is an
	// occurrence a coach tallies, so none reaches the counters below.
	if entry.Group == GroupControl {
		f.applyControl(e)
		return
	}

	if entry.Player {
		if _, selected := f.state.Players[e.PlayerID]; !selected {
			// Tagged against someone since removed from the lineup. Attributing it
			// anywhere would be a guess, so it is dropped.
			return
		}
		f.countForPlayer(e.PlayerID, e.Kind)
	}
	f.count(e.Kind, e.ClockMs)
	f.score(entry, e.ClockMs)

	switch e.Kind {
	case KindSub:
		f.substitute(e)
	case KindYellowCard:
		f.sinBin(e.PlayerID)
	case KindRedCard:
		f.sendOff(e.PlayerID)
	}
}

func (f *folder) applyControl(e Event) {
	switch e.Kind {
	case KindPeriodStarted, KindClockResumed:
		f.running = true
		f.state.Period = e.Period
	case KindClockPaused, KindPeriodEnded:
		f.running = false
	case KindMatchEnded:
		f.running = false
		f.state.Status = MatchCompleted
	case KindZoneChanged:
		// The toggle carries its whole state rather than a delta, so an unknown
		// value leaves the current setting alone instead of corrupting it.
		if zone := Zone(e.Payload[PayloadZone]); zone.Valid() {
			f.zone = zone
		}
		if possession := Possession(e.Payload[PayloadPossession]); possession.Valid() {
			f.possession = possession
		}
	}
}

func (f *folder) substitute(e Event) {
	on, off := e.Payload[PayloadOnPlayer], e.Payload[PayloadOffPlayer]
	if _, selected := f.state.Players[on]; !selected {
		return
	}

	// The outgoing player may already be off — carded, or replaced earlier — and
	// the incoming player still comes on.
	delete(f.onPitch, off)
	delete(f.binBudget, off)
	f.onPitch[on] = true
	f.inheritPosition(on, off)
}

// inheritPosition gives a replacement the position of whoever they came on for.
// A bench slot only carries a number, so without this a replacement has no
// comparison group and drops out of every position-group ranking. It applies only
// while the position is unset, so a player who comes on twice keeps the first.
func (f *folder) inheritPosition(on, off string) {
	outgoing, ok := f.state.Players[off]
	if !ok || outgoing.Position == "" {
		return
	}

	incoming := f.state.Players[on]
	if incoming.Position != "" {
		return
	}
	incoming.Position = outgoing.Position
	incoming.Group = outgoing.Group
	f.state.Players[on] = incoming
}

func (f *folder) sinBin(playerID string) {
	if f.sentOff[playerID] {
		return
	}
	delete(f.onPitch, playerID)
	f.binBudget[playerID] = sinBinMs
}

func (f *folder) sendOff(playerID string) {
	delete(f.onPitch, playerID)
	delete(f.binBudget, playerID)
	f.sentOff[playerID] = true
}

func (f *folder) count(kind EventKind, clockMs int) {
	f.state.Counts[kind]++
	f.state.slice(clockMs / SliceDurationMs).Counts[kind]++
}

func (f *folder) score(entry CatalogueEntry, clockMs int) {
	if entry.Points == 0 {
		return
	}

	bucket := f.state.slice(clockMs / SliceDurationMs)
	if isOppositionScore(entry) {
		f.state.Score.Them += entry.Points
		bucket.Score.Them += entry.Points
		return
	}
	f.state.Score.Us += entry.Points
	bucket.Score.Us += entry.Points
}

func (f *folder) countForPlayer(playerID string, kind EventKind) {
	stats, ok := f.state.Players[playerID]
	if !ok {
		return
	}
	// Counts is a map, so incrementing it reaches the stored entry without a
	// write-back. MinutesMs, being an int, does need one — see addMinutes.
	stats.Counts[kind]++
}

func (f *folder) addMinutes(playerID string, ms int) {
	stats, ok := f.state.Players[playerID]
	if !ok {
		return
	}
	stats.MinutesMs += ms
	f.state.Players[playerID] = stats
}

func (f *folder) finish() {
	f.state.ClockMs = f.lastClock
	f.state.Running = f.running
	f.state.Zone = f.zone
	f.state.Possession = f.possession

	for playerID, stats := range f.state.Players {
		stats.OnPitch = f.onPitch[playerID]
		f.state.Players[playerID] = stats
	}
}

// isOppositionScore identifies the one kind of scoring event that belongs to
// nobody: this application never holds the other team's squad.
func isOppositionScore(entry CatalogueEntry) bool {
	return entry.Group == GroupScore && !entry.Player
}

// addSpan attributes a duration to every 20-minute slice it overlaps, so a span
// running from 19:00 to 22:00 lands one minute in one bucket and two in the next.
func addSpan(fromMs, toMs int, add func(index, ms int)) {
	if fromMs < 0 {
		fromMs = 0
	}

	for cursor := fromMs; cursor < toMs; {
		index := cursor / SliceDurationMs
		end := (index + 1) * SliceDurationMs
		if end > toMs {
			end = toMs
		}
		add(index, end-cursor)
		cursor = end
	}
}
