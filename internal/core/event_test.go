package core

import (
	"strings"
	"testing"
)

// matchSquad is the selection an event is validated against. Two players is
// enough to prove both membership and the substitution pairing.
func matchSquad() map[string]bool {
	return map[string]bool{"player-1": true, "player-2": true}
}

func validEvent() Event {
	return Event{
		ID:       "0192f0c1-0000-7000-8000-000000000001",
		Kind:     KindTackleMade,
		PlayerID: "player-1",
		ClockMs:  600000,
		Period:   1,
		DeviceID: "device-1",
	}
}

func TestEventValidateAcceptsValidEvents(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Event)
		squad  map[string]bool
	}{
		{"a player action", func(*Event) {}, matchSquad()},
		{
			"a zone toggle",
			func(e *Event) {
				e.Kind = KindZoneChanged
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadZone:       string(ZoneTheir22),
					PayloadPossession: string(PossessionUs),
				}
			},
			matchSquad(),
		},
		{
			"a substitution",
			func(e *Event) {
				e.Kind = KindSub
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadOnPlayer:  "player-2",
					PayloadOffPlayer: "player-1",
				}
			},
			matchSquad(),
		},
		{
			"a void",
			func(e *Event) {
				e.Kind = KindVoid
				e.PlayerID = ""
				e.VoidsID = "0192f0c1-0000-7000-8000-000000000000"
			},
			matchSquad(),
		},
		{
			"a control event needs no squad, so the clock can run before an XV is named",
			func(e *Event) {
				e.Kind = KindPeriodStarted
				e.PlayerID = ""
				e.ClockMs = 0
			},
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := validEvent()
			c.mutate(&e)

			if err := e.Validate(c.squad); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestEventValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Event)
		field  string
	}{
		{"empty id", func(e *Event) { e.ID = " " }, "id"},
		{"empty device", func(e *Event) { e.DeviceID = "" }, "deviceId"},
		{"negative clock", func(e *Event) { e.ClockMs = -1 }, "clockMs"},
		{"period before the first", func(e *Event) { e.Period = 0 }, "period"},
		{"period after extra time", func(e *Event) { e.Period = 5 }, "period"},
		{"unknown kind", func(e *Event) { e.Kind = "tackle_almost" }, "kind"},
		{"group used as kind", func(e *Event) { e.Kind = EventKind(GroupPlay) }, "kind"},
		{"no player on a player action", func(e *Event) { e.PlayerID = "" }, "playerId"},
		{"player outside the squad", func(e *Event) { e.PlayerID = "player-99" }, "playerId"},
		{
			"player on a team-level event",
			func(e *Event) {
				e.Kind = KindZoneChanged
				e.Payload = map[string]string{
					PayloadZone:       string(ZoneOur22),
					PayloadPossession: string(PossessionThem),
				}
			},
			"playerId",
		},
		{
			"zone toggle with no payload",
			func(e *Event) {
				e.Kind = KindZoneChanged
				e.PlayerID = ""
			},
			PayloadZone,
		},
		{
			"zone toggle with an unknown zone",
			func(e *Event) {
				e.Kind = KindZoneChanged
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadZone:       "halfway",
					PayloadPossession: string(PossessionUs),
				}
			},
			PayloadZone,
		},
		{
			"zone toggle with an unknown possession",
			func(e *Event) {
				e.Kind = KindZoneChanged
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadZone:       string(ZoneOurHalf),
					PayloadPossession: "contested",
				}
			},
			PayloadPossession,
		},
		{
			"substitution missing the outgoing player",
			func(e *Event) {
				e.Kind = KindSub
				e.PlayerID = ""
				e.Payload = map[string]string{PayloadOnPlayer: "player-2"}
			},
			PayloadOffPlayer,
		},
		{
			"substitution naming someone outside the squad",
			func(e *Event) {
				e.Kind = KindSub
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadOnPlayer:  "player-99",
					PayloadOffPlayer: "player-1",
				}
			},
			PayloadOnPlayer,
		},
		{
			"substitution of a player for themselves",
			func(e *Event) {
				e.Kind = KindSub
				e.PlayerID = ""
				e.Payload = map[string]string{
					PayloadOnPlayer:  "player-1",
					PayloadOffPlayer: "player-1",
				}
			},
			PayloadOnPlayer,
		},
		{
			"void naming nothing",
			func(e *Event) {
				e.Kind = KindVoid
				e.PlayerID = ""
			},
			"voidsId",
		},
		{
			"a tagged event cancelling another",
			func(e *Event) { e.VoidsID = "0192f0c1-0000-7000-8000-000000000000" },
			"voidsId",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := validEvent()
			c.mutate(&e)

			err := e.Validate(matchSquad())
			if err == nil {
				t.Fatalf("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestZoneAndPossessionValid(t *testing.T) {
	for _, zone := range AllZones() {
		if !zone.Valid() {
			t.Errorf("Zone(%q).Valid() = false, want true", zone)
		}
	}
	for _, bad := range []Zone{"", "halfway", "our_half_ish", Zone(PossessionUs)} {
		if bad.Valid() {
			t.Errorf("Zone(%q).Valid() = true, want false", bad)
		}
	}

	for _, possession := range AllPossessions() {
		if !possession.Valid() {
			t.Errorf("Possession(%q).Valid() = false, want true", possession)
		}
	}
	for _, bad := range []Possession{"", "ours", "contested"} {
		if bad.Valid() {
			t.Errorf("Possession(%q).Valid() = true, want false", bad)
		}
	}
}
