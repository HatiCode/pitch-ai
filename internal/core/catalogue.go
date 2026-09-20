package core

// EventKind identifies one kind of tagged occurrence. The set is closed and lives
// here, so a new statistic is a new kind plus a case in a pure function, with
// nothing in store/ or httpapi/ changing.
type EventKind string

// PLAY — always visible, high frequency. Thirteen kinds is the practical ceiling
// for tapping without looking, which is why clearout and jackal sit here despite
// being breakdown events: they are too frequent to hide behind a tab.
const (
	KindTackleMade     EventKind = "tackle_made"
	KindTackleMissed   EventKind = "tackle_missed"
	KindCarry          EventKind = "carry"
	KindLinebreak      EventKind = "linebreak"
	KindDefenderBeaten EventKind = "defender_beaten"
	KindOffload        EventKind = "offload"
	KindClearout       EventKind = "clearout"
	KindJackalWon      EventKind = "jackal_won"
	KindTurnoverWon    EventKind = "turnover_won"
	KindHandlingError  EventKind = "handling_error"
	KindKick           EventKind = "kick"
	KindTry            EventKind = "try"
	KindSub            EventKind = "sub"
)

// SET PIECE — occurs at natural stoppages, where one extra tap is affordable.
const (
	KindScrumWon             EventKind = "scrum_won"
	KindScrumLost            EventKind = "scrum_lost"
	KindScrumPenaltyWon      EventKind = "scrum_penalty_won"
	KindScrumPenaltyConceded EventKind = "scrum_penalty_conceded"
	KindLineoutWon           EventKind = "lineout_won"
	KindLineoutLost          EventKind = "lineout_lost"
	KindThrowNotStraight     EventKind = "throw_not_straight"
	KindMaulFromLineout      EventKind = "maul_from_lineout"
	KindRestartReceived      EventKind = "restart_received"
	KindRestartLost          EventKind = "restart_lost"
)

// DISCIPLINE — the two cards have side effects in the fold: without them minutes
// played silently lies, and every per-80 rate built on it lies too.
const (
	KindOffside      EventKind = "offside"
	KindRuckOffence  EventKind = "ruck_offence"
	KindHighTackle   EventKind = "high_tackle"
	KindNotReleasing EventKind = "not_releasing"
	KindScrumOffence EventKind = "scrum_offence"
	KindFoulPlay     EventKind = "foul_play"
	KindYellowCard   EventKind = "yellow_card"
	KindRedCard      EventKind = "red_card"
)

// SCORE — a try alone cannot produce a scoreboard, so the kicks and the
// opposition's scores are here. They render as a strip beside the score rather
// than a fourth tab, because they happen at stoppages and the tabs are full.
const (
	KindConversionMade       EventKind = "conversion_made"
	KindConversionMissed     EventKind = "conversion_missed"
	KindPenaltyGoal          EventKind = "penalty_goal"
	KindDropGoal             EventKind = "drop_goal"
	KindOppositionTry        EventKind = "opposition_try"
	KindOppositionConversion EventKind = "opposition_conversion"
	KindOppositionPenalty    EventKind = "opposition_penalty"
	KindOppositionDrop       EventKind = "opposition_drop"
)

// CONTROL — the clock and the log's own bookkeeping. No buttons on a tab.
//
// The clock is driven by these events rather than by device state, so it survives
// a reload and two coaches tagging one match agree on it without a merge rule.
const (
	KindPeriodStarted EventKind = "period_started"
	KindClockPaused   EventKind = "clock_paused"
	KindClockResumed  EventKind = "clock_resumed"
	KindPeriodEnded   EventKind = "period_ended"
	KindMatchEnded    EventKind = "match_ended"
	KindZoneChanged   EventKind = "zone_changed"
	KindVoid          EventKind = "void"
)

// EventGroup is where a kind appears in the tagging screen.
type EventGroup string

const (
	GroupPlay       EventGroup = "play"
	GroupSetPiece   EventGroup = "set_piece"
	GroupDiscipline EventGroup = "discipline"
	GroupScore      EventGroup = "score"
	GroupControl    EventGroup = "control"
)

// AllEventGroups returns every group in the order the screen lays them out.
func AllEventGroups() []EventGroup {
	return []EventGroup{GroupPlay, GroupSetPiece, GroupDiscipline, GroupScore, GroupControl}
}

// CatalogueEntry is the configuration for one kind: what it is called, where it
// appears, whether it belongs to a player, and what it is worth.
//
// This is served to the client rather than hard-coded in the frontend, so adding
// a kind is a data change and a redeploy of one binary — and so a second club can
// eventually be given a different catalogue without a fork.
type CatalogueEntry struct {
	Kind   EventKind  `json:"kind"`
	Label  string     `json:"label"`
	Group  EventGroup `json:"group"`
	Player bool       `json:"player"`
	Points int        `json:"points,omitempty"`
}

// Catalogue returns every event kind in the order its buttons are drawn.
//
// Player attribution is a tagging-cost decision as much as a statistical one. A
// scrum outcome belongs to eight players and a maul to the whole pack, so those
// stay team-level; a lineout has a jumper and a restart has a receiver, and both
// are individual skills a coach selects on, so those name a player.
func Catalogue() []CatalogueEntry {
	return []CatalogueEntry{
		{Kind: KindTackleMade, Label: "Tackle made", Group: GroupPlay, Player: true},
		{Kind: KindTackleMissed, Label: "Tackle missed", Group: GroupPlay, Player: true},
		{Kind: KindCarry, Label: "Carry", Group: GroupPlay, Player: true},
		{Kind: KindLinebreak, Label: "Linebreak", Group: GroupPlay, Player: true},
		{Kind: KindDefenderBeaten, Label: "Defender beaten", Group: GroupPlay, Player: true},
		{Kind: KindOffload, Label: "Offload", Group: GroupPlay, Player: true},
		{Kind: KindClearout, Label: "Clearout", Group: GroupPlay, Player: true},
		{Kind: KindJackalWon, Label: "Jackal won", Group: GroupPlay, Player: true},
		{Kind: KindTurnoverWon, Label: "Turnover won", Group: GroupPlay, Player: true},
		{Kind: KindHandlingError, Label: "Handling error", Group: GroupPlay, Player: true},
		{Kind: KindKick, Label: "Kick", Group: GroupPlay, Player: true},
		{Kind: KindTry, Label: "Try", Group: GroupPlay, Player: true, Points: 5},
		{Kind: KindSub, Label: "Substitution", Group: GroupPlay},

		{Kind: KindScrumWon, Label: "Scrum won", Group: GroupSetPiece},
		{Kind: KindScrumLost, Label: "Scrum lost", Group: GroupSetPiece},
		{Kind: KindScrumPenaltyWon, Label: "Scrum penalty won", Group: GroupSetPiece},
		{Kind: KindScrumPenaltyConceded, Label: "Scrum penalty conceded", Group: GroupSetPiece},
		{Kind: KindLineoutWon, Label: "Lineout won", Group: GroupSetPiece, Player: true},
		{Kind: KindLineoutLost, Label: "Lineout lost", Group: GroupSetPiece, Player: true},
		{Kind: KindThrowNotStraight, Label: "Throw not straight", Group: GroupSetPiece, Player: true},
		{Kind: KindMaulFromLineout, Label: "Maul from lineout", Group: GroupSetPiece},
		{Kind: KindRestartReceived, Label: "Restart received", Group: GroupSetPiece, Player: true},
		{Kind: KindRestartLost, Label: "Restart lost", Group: GroupSetPiece},

		{Kind: KindOffside, Label: "Offside", Group: GroupDiscipline, Player: true},
		{Kind: KindRuckOffence, Label: "Ruck offence", Group: GroupDiscipline, Player: true},
		{Kind: KindHighTackle, Label: "High tackle", Group: GroupDiscipline, Player: true},
		{Kind: KindNotReleasing, Label: "Not releasing", Group: GroupDiscipline, Player: true},
		{Kind: KindScrumOffence, Label: "Scrum offence", Group: GroupDiscipline, Player: true},
		{Kind: KindFoulPlay, Label: "Foul play", Group: GroupDiscipline, Player: true},
		{Kind: KindYellowCard, Label: "Yellow card", Group: GroupDiscipline, Player: true},
		{Kind: KindRedCard, Label: "Red card", Group: GroupDiscipline, Player: true},

		{Kind: KindConversionMade, Label: "Conversion", Group: GroupScore, Player: true, Points: 2},
		{Kind: KindConversionMissed, Label: "Conversion missed", Group: GroupScore, Player: true},
		{Kind: KindPenaltyGoal, Label: "Penalty goal", Group: GroupScore, Player: true, Points: 3},
		{Kind: KindDropGoal, Label: "Drop goal", Group: GroupScore, Player: true, Points: 3},
		{Kind: KindOppositionTry, Label: "Their try", Group: GroupScore, Points: 5},
		{Kind: KindOppositionConversion, Label: "Their conversion", Group: GroupScore, Points: 2},
		{Kind: KindOppositionPenalty, Label: "Their penalty", Group: GroupScore, Points: 3},
		{Kind: KindOppositionDrop, Label: "Their drop goal", Group: GroupScore, Points: 3},

		{Kind: KindPeriodStarted, Label: "Start period", Group: GroupControl},
		{Kind: KindClockPaused, Label: "Stop clock", Group: GroupControl},
		{Kind: KindClockResumed, Label: "Start clock", Group: GroupControl},
		{Kind: KindPeriodEnded, Label: "End period", Group: GroupControl},
		{Kind: KindMatchEnded, Label: "End match", Group: GroupControl},
		{Kind: KindZoneChanged, Label: "Zone changed", Group: GroupControl},
		{Kind: KindVoid, Label: "Undo", Group: GroupControl},
	}
}

// AllEventKinds returns every kind, derived from the catalogue so the two cannot
// disagree.
func AllEventKinds() []EventKind {
	entries := Catalogue()
	kinds := make([]EventKind, len(entries))
	for i, entry := range entries {
		kinds[i] = entry.Kind
	}
	return kinds
}

// catalogueByKind indexes the catalogue once, at package initialisation, because
// every appended event looks its kind up.
var catalogueByKind = func() map[EventKind]CatalogueEntry {
	entries := Catalogue()
	byKind := make(map[EventKind]CatalogueEntry, len(entries))
	for _, entry := range entries {
		byKind[entry.Kind] = entry
	}
	return byKind
}()

// LookupKind returns the catalogue entry for a kind. An unknown kind is not an
// error here; callers decide whether to reject it or skip it.
func LookupKind(kind EventKind) (CatalogueEntry, bool) {
	entry, ok := catalogueByKind[kind]
	return entry, ok
}
