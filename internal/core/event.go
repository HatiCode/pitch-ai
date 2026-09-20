package core

import (
	"slices"
	"strconv"
	"strings"
)

// Zone is the part of the pitch play is in, seen from the tagging team's end. It
// is a running toggle rather than a location tag on every event: time
// accumulates in whichever zone is current, which yields a territory figure for
// almost no tagging effort.
type Zone string

const (
	ZoneOur22     Zone = "our_22"
	ZoneOurHalf   Zone = "our_half"
	ZoneTheirHalf Zone = "their_half"
	ZoneTheir22   Zone = "their_22"
)

// Possession is the other running toggle: who has the ball.
type Possession string

const (
	PossessionUs   Possession = "us"
	PossessionThem Possession = "them"
)

// AllZones returns every zone in pitch order, for building the toggle strip.
func AllZones() []Zone {
	return []Zone{ZoneOur22, ZoneOurHalf, ZoneTheirHalf, ZoneTheir22}
}

func AllPossessions() []Possession {
	return []Possession{PossessionUs, PossessionThem}
}

func (z Zone) Valid() bool {
	return slices.Contains(AllZones(), z)
}

func (p Possession) Valid() bool {
	return slices.Contains(AllPossessions(), p)
}

// Payload keys. Payload values are always strings because the map round-trips
// through JSON in two languages, and a number would arrive as a float64 on one
// side and an int on the other.
const (
	PayloadZone       = "zone"
	PayloadPossession = "possession"
	PayloadOnPlayer   = "onPlayerId"
	PayloadOffPlayer  = "offPlayerId"
)

// Periods 1 and 2 are the halves; 3 and 4 are extra time.
const (
	firstPeriod = 1
	lastPeriod  = 4
)

// Event is one immutable entry in a match's log. Nothing is ever mutated or
// deleted: a mis-tap is cancelled by appending a void that references it, which
// keeps every derived figure recomputable from scratch and leaves an audit trail
// of what was corrected after the match.
type Event struct {
	ID       string            `json:"id" firestore:"-"`
	Kind     EventKind         `json:"kind" firestore:"kind"`
	PlayerID string            `json:"playerId,omitempty" firestore:"playerId"`
	ClockMs  int               `json:"clockMs" firestore:"clockMs"`
	Period   int               `json:"period" firestore:"period"`
	Payload  map[string]string `json:"payload,omitempty" firestore:"payload"`
	DeviceID string            `json:"deviceId" firestore:"deviceId"`
	VoidsID  string            `json:"voidsId,omitempty" firestore:"voidsId"`
}

// Validate checks the event against the catalogue and against the squad selected
// for this match. An event naming a player who is not in the lineup would hand
// that player minutes they never played, so it is refused on arrival rather than
// discovered in a season report.
//
// A control event belongs to no player and so needs no squad: the clock can run
// before an XV is named.
func (e Event) Validate(squad map[string]bool) error {
	if strings.TrimSpace(e.ID) == "" {
		return ValidationError{Field: "id", Reason: "must not be empty"}
	}
	if strings.TrimSpace(e.DeviceID) == "" {
		return ValidationError{Field: "deviceId", Reason: "must not be empty"}
	}
	if e.ClockMs < 0 {
		return ValidationError{Field: "clockMs", Reason: "must not be negative"}
	}
	if e.Period < firstPeriod || e.Period > lastPeriod {
		return ValidationError{
			Field:  "period",
			Reason: "must be between " + strconv.Itoa(firstPeriod) + " and " + strconv.Itoa(lastPeriod),
		}
	}

	entry, ok := LookupKind(e.Kind)
	if !ok {
		return ValidationError{Field: "kind", Reason: "unknown event kind " + string(e.Kind)}
	}
	if err := e.validatePlayer(entry, squad); err != nil {
		return err
	}
	if err := e.validateVoid(); err != nil {
		return err
	}
	return e.validatePayload(squad)
}

func (e Event) validatePlayer(entry CatalogueEntry, squad map[string]bool) error {
	if !entry.Player {
		if e.PlayerID != "" {
			return ValidationError{
				Field:  "playerId",
				Reason: string(e.Kind) + " is a team-level event and takes no player",
			}
		}
		return nil
	}
	if e.PlayerID == "" {
		return ValidationError{Field: "playerId", Reason: string(e.Kind) + " must name a player"}
	}
	if !squad[e.PlayerID] {
		return ValidationError{Field: "playerId", Reason: "player " + e.PlayerID + " is not in this match squad"}
	}
	return nil
}

func (e Event) validateVoid() error {
	if e.Kind == KindVoid && e.VoidsID == "" {
		return ValidationError{Field: "voidsId", Reason: "a void must name the event it cancels"}
	}
	if e.Kind != KindVoid && e.VoidsID != "" {
		return ValidationError{Field: "voidsId", Reason: "only a void event cancels another"}
	}
	return nil
}

// validatePayload covers the two kinds carrying structured payloads. A zone
// toggle carries the whole toggle state rather than a delta, so replaying the log
// from any point lands on the same zone.
func (e Event) validatePayload(squad map[string]bool) error {
	switch e.Kind {
	case KindZoneChanged:
		if zone := Zone(e.Payload[PayloadZone]); !zone.Valid() {
			return ValidationError{
				Field:  PayloadZone,
				Reason: "must be one of our_22, our_half, their_half, their_22",
			}
		}
		if possession := Possession(e.Payload[PayloadPossession]); !possession.Valid() {
			return ValidationError{Field: PayloadPossession, Reason: "must be us or them"}
		}
	case KindSub:
		on, err := e.payloadSquadMember(PayloadOnPlayer, squad)
		if err != nil {
			return err
		}
		off, err := e.payloadSquadMember(PayloadOffPlayer, squad)
		if err != nil {
			return err
		}
		if on == off {
			return ValidationError{
				Field:  PayloadOnPlayer,
				Reason: "a substitution must name two different players",
			}
		}
	}
	return nil
}

func (e Event) payloadSquadMember(field string, squad map[string]bool) (string, error) {
	id := e.Payload[field]
	if id == "" {
		return "", ValidationError{Field: field, Reason: "must name a player"}
	}
	if !squad[id] {
		return "", ValidationError{Field: field, Reason: "player " + id + " is not in this match squad"}
	}
	return id, nil
}
