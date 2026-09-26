package firestore

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

func appendEvents(t *testing.T, store *Store, clubID, teamID, matchID string, events ...core.Event) {
	t.Helper()
	if err := store.AppendEvents(context.Background(), clubID, teamID, matchID, events); err != nil {
		t.Fatalf("AppendEvents() error = %v", err)
	}
}

func tackleMade(id, playerID string, clockMs int) core.Event {
	return core.Event{
		ID:       id,
		Kind:     core.KindTackleMade,
		PlayerID: playerID,
		ClockMs:  clockMs,
		Period:   1,
		DeviceID: "device-1",
	}
}

// seedEventAt writes an event document with an arrival time of its own choosing,
// which AppendEvents cannot do: its recordedAt is assigned by the server. Pinning
// the timestamp is the only way to test a tie without waiting for two writes to
// land in the same microsecond by luck.
func seedEventAt(t *testing.T, store *Store, clubID, teamID, matchID string, e core.Event, recordedAt time.Time) {
	t.Helper()

	doc := newEventDoc(e)
	_, err := store.eventsCol(clubID, teamID, matchID).Doc(e.ID).Set(context.Background(), map[string]any{
		"kind":       doc.Kind,
		"playerId":   doc.PlayerID,
		"clockMs":    doc.ClockMs,
		"period":     doc.Period,
		"payload":    doc.Payload,
		"deviceId":   doc.DeviceID,
		"voidsId":    doc.VoidsID,
		"recordedAt": recordedAt,
	})
	if err != nil {
		t.Fatalf("seeding event %q: %v", e.ID, err)
	}
}

func eventsByID(events []core.Event) map[string]core.Event {
	byID := make(map[string]core.Event, len(events))
	for _, e := range events {
		byID[e.ID] = e
	}
	return byID
}

func eventIDs(events []core.Event) []string {
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	return ids
}

func readEvents(t *testing.T, store *Store, clubID, teamID, matchID, since string) ([]core.Event, string) {
	t.Helper()
	events, cursor, err := store.Events(context.Background(), clubID, teamID, matchID, since)
	if err != nil {
		t.Fatalf("Events(since=%q) error = %v", since, err)
	}
	return events, cursor
}

func TestAppendedEventsReadBackIntact(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	zone := core.Event{
		ID:      "e-zone",
		Kind:    core.KindZoneChanged,
		ClockMs: 120000,
		Period:  1,
		Payload: map[string]string{
			core.PayloadZone:       string(core.ZoneTheir22),
			core.PayloadPossession: string(core.PossessionUs),
		},
		DeviceID: "device-2",
	}
	void := core.Event{
		ID:       "e-void",
		Kind:     core.KindVoid,
		ClockMs:  130000,
		Period:   1,
		DeviceID: "device-2",
		VoidsID:  "e-tackle",
	}

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e-tackle", "p7", 60000), zone, void)

	got, _ := readEvents(t, store, clubID, "team-1", "m1", "")
	if len(got) != 3 {
		t.Fatalf("len(Events()) = %d, want 3", len(got))
	}
	byID := eventsByID(got)

	tackle := byID["e-tackle"]
	if tackle.Kind != core.KindTackleMade || tackle.PlayerID != "p7" || tackle.ClockMs != 60000 {
		t.Errorf("tackle = %+v, want tackle_made by p7 at 60000", tackle)
	}
	if tackle.Period != 1 || tackle.DeviceID != "device-1" {
		t.Errorf("tackle period/device = (%d, %q), want (1, device-1)", tackle.Period, tackle.DeviceID)
	}
	// An event with no payload must come back empty rather than as a map with a
	// zero-valued key: the fold reads payload keys by presence.
	if len(tackle.Payload) != 0 {
		t.Errorf("tackle.Payload = %v, want empty", tackle.Payload)
	}
	if tackle.VoidsID != "" {
		t.Errorf("tackle.VoidsID = %q, want empty", tackle.VoidsID)
	}

	if got := byID["e-zone"].Payload; got[core.PayloadZone] != string(core.ZoneTheir22) ||
		got[core.PayloadPossession] != string(core.PossessionUs) {
		t.Errorf("zone payload = %v, want their_22/us", got)
	}
	if got := byID["e-void"].VoidsID; got != "e-tackle" {
		t.Errorf("void.VoidsID = %q, want e-tackle", got)
	}
}

// The append endpoint is idempotent, so a client that retries after a flaky
// response must not double a count.
func TestAppendingTheSameEventTwiceLeavesOneDocument(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 1000))
	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 2000))

	got, _ := readEvents(t, store, clubID, "team-1", "m1", "")
	if len(got) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(got))
	}
	if got[0].ClockMs != 2000 {
		t.Errorf("ClockMs = %d, want 2000 (the later write wins)", got[0].ClockMs)
	}
}

func TestEventsWithNoCursorReturnsTheWholeLogInAppendOrder(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 1000))
	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e2", "p8", 2000))
	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e3", "p9", 3000))

	got, cursor := readEvents(t, store, clubID, "team-1", "m1", "")
	if want := []string{"e1", "e2", "e3"}; !slices.Equal(eventIDs(got), want) {
		t.Errorf("event IDs = %v, want %v", eventIDs(got), want)
	}
	if cursor == "" {
		t.Error("cursor = empty, want a cursor to resume from")
	}
}

func TestEventsResumesFromTheCursor(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 1000), tackleMade("e2", "p8", 2000))
	first, cursor := readEvents(t, store, clubID, "team-1", "m1", "")
	if len(first) != 2 {
		t.Fatalf("len(Events()) = %d, want 2", len(first))
	}

	// An idle match must not rewind the client to the start of the log.
	empty, idleCursor := readEvents(t, store, clubID, "team-1", "m1", cursor)
	if len(empty) != 0 {
		t.Errorf("Events(since=cursor) returned %d events, want 0", len(empty))
	}
	if idleCursor != cursor {
		t.Errorf("cursor after an empty page = %q, want it unchanged (%q)", idleCursor, cursor)
	}

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e3", "p9", 3000))
	next, nextCursor := readEvents(t, store, clubID, "team-1", "m1", cursor)
	if want := []string{"e3"}; !slices.Equal(eventIDs(next), want) {
		t.Errorf("event IDs since cursor = %v, want %v", eventIDs(next), want)
	}
	if nextCursor == cursor {
		t.Error("cursor did not advance after a new event")
	}
}

// Two events can share an arrival timestamp — two devices tagging one match, or
// two writes committed together — and then the document ID is the only thing
// separating them. A cursor that ignored it would replay one of them forever or
// skip the other entirely.
func TestEventsSharingATimestampSurviveACursorRoundTrip(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)
	arrived := time.Date(2026, 3, 14, 15, 22, 11, 0, time.UTC)

	seedEventAt(t, store, clubID, "team-1", "m1", tackleMade("b-late", "p7", 2000), arrived)
	seedEventAt(t, store, clubID, "team-1", "m1", tackleMade("a-early", "p8", 1000), arrived)

	got, cursor := readEvents(t, store, clubID, "team-1", "m1", "")
	if want := []string{"a-early", "b-late"}; !slices.Equal(eventIDs(got), want) {
		t.Errorf("event IDs = %v, want %v (document ID breaks the timestamp tie)", eventIDs(got), want)
	}

	after, _ := readEvents(t, store, clubID, "team-1", "m1", cursor)
	if len(after) != 0 {
		t.Errorf("Events(since=cursor) returned %v, want nothing", eventIDs(after))
	}
}

// A batch holding one ID twice is a client that retried into its own outbox. It
// must land as one event rather than fail the batch.
func TestDuplicateIDsWithinOneBatchCollapse(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 1000), tackleMade("e1", "p7", 2000))

	got, _ := readEvents(t, store, clubID, "team-1", "m1", "")
	if len(got) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(got))
	}
	if got[0].ClockMs != 2000 {
		t.Errorf("ClockMs = %d, want 2000 (the later copy wins)", got[0].ClockMs)
	}
}

func TestMalformedCursorIsAValidationError(t *testing.T) {
	store := newTestStore(t)

	cursors := map[string]string{
		"no separator":      "1758288000000000",
		"no event id":       "1758288000000000:",
		"no timestamp":      ":e1",
		"timestamp is text": "yesterday:e1",
	}

	for name, cursor := range cursors {
		t.Run(name, func(t *testing.T) {
			_, _, err := store.Events(context.Background(), uniqueClubID(t), "team-1", "m1", cursor)

			var invalid core.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Events(since=%q) error = %v, want core.ValidationError", cursor, err)
			}
			if invalid.Field != "since" {
				t.Errorf("ValidationError.Field = %q, want %q", invalid.Field, "since")
			}
		})
	}
}

func TestEventsAreScopedToTheirMatch(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)

	appendEvents(t, store, clubID, "team-1", "m1", tackleMade("e1", "p7", 1000))
	appendEvents(t, store, clubID, "team-1", "m2", tackleMade("e1", "p7", 5000), tackleMade("e2", "p8", 6000))

	got, _ := readEvents(t, store, clubID, "team-1", "m1", "")
	if want := []string{"e1"}; !slices.Equal(eventIDs(got), want) {
		t.Errorf("m1 event IDs = %v, want %v", eventIDs(got), want)
	}
	if got[0].ClockMs != 1000 {
		t.Errorf("m1 ClockMs = %d, want 1000 — the other match's event of the same ID leaked in", got[0].ClockMs)
	}
}
