package firestore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"pitch-ai/internal/core"
)

// eventsCol is a subcollection of the match, so an event can never be read
// without its club and team in the path.
func (s *Store) eventsCol(clubID, teamID, matchID string) *fs.CollectionRef {
	return s.matchesCol(clubID, teamID).Doc(matchID).Collection("events")
}

// eventDoc is the stored shape. It carries recordedAt, which core.Event does not:
// server-assigned arrival order is what a client's `since` cursor walks, and that
// is sync bookkeeping rather than domain data the fold would ever read.
//
// RecordedAt is always left zero when writing. The serverTimestamp option only
// substitutes the server's time into a zero field; a non-zero one is dropped from
// the write entirely, which under Set would erase the ordering the cursor depends
// on.
type eventDoc struct {
	Kind       core.EventKind    `firestore:"kind"`
	PlayerID   string            `firestore:"playerId"`
	ClockMs    int               `firestore:"clockMs"`
	Period     int               `firestore:"period"`
	Payload    map[string]string `firestore:"payload"`
	DeviceID   string            `firestore:"deviceId"`
	VoidsID    string            `firestore:"voidsId"`
	RecordedAt time.Time         `firestore:"recordedAt,serverTimestamp"`
}

func newEventDoc(e core.Event) eventDoc {
	return eventDoc{
		Kind:     e.Kind,
		PlayerID: e.PlayerID,
		ClockMs:  e.ClockMs,
		Period:   e.Period,
		Payload:  e.Payload,
		DeviceID: e.DeviceID,
		VoidsID:  e.VoidsID,
	}
}

// event restores the domain event. The ID lives in the document path rather than
// the body, the same way players and matches carry theirs.
func (d eventDoc) event(id string) core.Event {
	return core.Event{
		ID:       id,
		Kind:     d.Kind,
		PlayerID: d.PlayerID,
		ClockMs:  d.ClockMs,
		Period:   d.Period,
		Payload:  d.Payload,
		DeviceID: d.DeviceID,
		VoidsID:  d.VoidsID,
	}
}

// AppendEvents writes a batch of events, keyed by the ID the tagging device
// generated.
//
// Set rather than Create: a retry after a flaky response must overwrite identical
// data rather than fail, which is the whole reason the endpoint is idempotent and
// why a coach on a sideline can retry blindly. A retry does move recordedAt later,
// so a client may be handed an event it has already seen — harmless, because the
// client upserts by ID too. What it can never do is move an event *behind* a
// cursor, so nothing is ever skipped.
//
// A BulkWriter rather than the deprecated WriteBatch, and the lost atomicity
// costs nothing here: a batch that half lands is indistinguishable from one the
// client had not finished sending, and the retry that follows converges on the
// same documents.
func (s *Store) AppendEvents(ctx context.Context, clubID, teamID, matchID string, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}

	// One ID twice in a batch is one event twice — a client that retried into its
	// own outbox. The later copy wins, exactly as a second append would. Passing
	// both through would fail the whole batch, because a BulkWriter refuses two
	// writes to one path, and that is not an error a coach on a sideline can act
	// on.
	latest := make(map[string]core.Event, len(events))
	for _, e := range events {
		latest[e.ID] = e
	}

	col := s.eventsCol(clubID, teamID, matchID)
	writer := s.client.BulkWriter(ctx)
	jobs := make([]*fs.BulkWriterJob, 0, len(latest))

	for id, e := range latest {
		job, err := writer.Set(col.Doc(id), newEventDoc(e))
		if err != nil {
			return fmt.Errorf("firestore: queueing event %q: %w", id, err)
		}
		jobs = append(jobs, job)
	}
	writer.End()

	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return fmt.Errorf("firestore: append events to match %q: %w", matchID, err)
		}
	}
	return nil
}

// Events returns the log in arrival order, starting after the given cursor. An
// empty cursor reads the whole log.
//
// The ordering is (recordedAt, document ID). Two events written in one batch share
// a commit timestamp, so the document ID is what separates them — without it a
// cursor would either replay one of them forever or skip the other. Both orderings
// are declared explicitly: the server would tie-break on __name__ by itself, but
// the client requires one cursor value per declared OrderBy. Neither field needs a
// composite index.
func (s *Store) Events(ctx context.Context, clubID, teamID, matchID, since string) ([]core.Event, string, error) {
	query := s.eventsCol(clubID, teamID, matchID).
		OrderBy("recordedAt", fs.Asc).
		OrderBy(fs.DocumentID, fs.Asc)

	if since != "" {
		recordedAt, id, err := decodeCursor(since)
		if err != nil {
			return nil, "", err
		}
		query = query.StartAfter(recordedAt, id)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var out []core.Event
	// An empty page returns the cursor it was given, so a client polling an idle
	// match is never rewound to the start of the log.
	cursor := since
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, cursor, nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("firestore: list events of match %q: %w", matchID, err)
		}

		var doc eventDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, "", fmt.Errorf("firestore: decode event %q: %w", snap.Ref.ID, err)
		}
		out = append(out, doc.event(snap.Ref.ID))
		cursor = encodeCursor(doc.RecordedAt, snap.Ref.ID)
	}
}

// cursorSep separates the two halves of a cursor. The split is on the first
// separator and the left half is always digits, so an event ID containing one
// still decodes whole.
const cursorSep = ":"

// encodeCursor renders the ordering pair Firestore resumes from. Microseconds
// rather than nanoseconds because that is Firestore's own timestamp resolution — a
// nanosecond cursor would claim a precision the store does not have.
func encodeCursor(recordedAt time.Time, id string) string {
	return strconv.FormatInt(recordedAt.UnixMicro(), 10) + cursorSep + id
}

func decodeCursor(cursor string) (time.Time, string, error) {
	malformed := core.ValidationError{Field: "since", Reason: "malformed cursor"}

	micros, id, found := strings.Cut(cursor, cursorSep)
	if !found || id == "" {
		return time.Time{}, "", malformed
	}
	parsed, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, "", malformed
	}
	return time.UnixMicro(parsed).UTC(), id, nil
}
