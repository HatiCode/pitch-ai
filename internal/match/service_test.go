package match

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

const (
	clubID  = "club-1"
	teamID  = "team-1"
	matchID = "match-1"
)

func playerID(jersey int) string { return "p" + strconv.Itoa(jersey) }

// taggedMatch is a scheduled fixture with a full selection, which is the state a
// match is in when a coach opens the tagging screen.
func taggedMatch() core.Match {
	var lineup core.Lineup
	for jersey := 1; jersey <= 15; jersey++ {
		lineup.Starters = append(lineup.Starters, core.LineupSlot{Jersey: jersey, PlayerID: playerID(jersey)})
	}
	for jersey := 16; jersey <= 23; jersey++ {
		lineup.Bench = append(lineup.Bench, core.LineupSlot{Jersey: jersey, PlayerID: playerID(jersey)})
	}

	return core.Match{
		ID:        matchID,
		ClubID:    clubID,
		TeamID:    teamID,
		SeasonID:  "2026-27",
		Opponent:  "Lansdowne",
		KickoffAt: time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC),
		Venue:     core.VenueHome,
		Status:    core.MatchScheduled,
		Lineup:    lineup,
	}
}

func newService() (*Service, *memStore) {
	store := newMemStore()
	store.matches[key(clubID, teamID, matchID)] = taggedMatch()
	return NewService(store, store, store, store, store), store
}

func eventID(n int) string {
	return fmt.Sprintf("0192f200-0000-7000-8000-%012x", n)
}

func tagged(n int, kind core.EventKind, clockMs int, player string) core.Event {
	return core.Event{
		ID:       eventID(n),
		Kind:     kind,
		PlayerID: player,
		ClockMs:  clockMs,
		Period:   1,
		DeviceID: "device-1",
	}
}

func control(n int, kind core.EventKind, clockMs int) core.Event {
	return core.Event{ID: eventID(n), Kind: kind, ClockMs: clockMs, Period: 1, DeviceID: "device-1"}
}

func TestAppendRejectsAPlayerOutsideTheSelection(t *testing.T) {
	svc, store := newService()

	events := []core.Event{tagged(1, core.KindTackleMade, 60000, "p99")}

	_, err := svc.Append(context.Background(), clubID, teamID, matchID, events)
	if err == nil {
		t.Fatal("Append() error = nil, want a validation error")
	}
	var invalid core.ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("error = %v, want a core.ValidationError", err)
	}
	if store.storedEventCount(clubID, teamID, matchID) != 0 {
		t.Error("a rejected batch was written")
	}
}

func TestAppendRejectsAnUnknownKind(t *testing.T) {
	svc, store := newService()

	events := []core.Event{tagged(1, "tackle_almost", 60000, playerID(7))}

	_, err := svc.Append(context.Background(), clubID, teamID, matchID, events)
	var invalid core.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want a core.ValidationError", err)
	}
	if store.storedEventCount(clubID, teamID, matchID) != 0 {
		t.Error("a rejected batch was written")
	}
}

// One bad event rejects the whole batch. A half-applied batch is indistinguishable
// from a partial sync and would leave the client retrying events forever.
func TestAppendRejectsTheWholeBatchOnOneBadEvent(t *testing.T) {
	svc, store := newService()

	events := []core.Event{
		tagged(1, core.KindTackleMade, 60000, playerID(7)),
		tagged(2, core.KindTackleMade, 120000, "p99"),
		tagged(3, core.KindCarry, 180000, playerID(12)),
	}

	if _, err := svc.Append(context.Background(), clubID, teamID, matchID, events); err == nil {
		t.Fatal("Append() error = nil, want a validation error")
	}
	if got := store.storedEventCount(clubID, teamID, matchID); got != 0 {
		t.Errorf("%d events were written, want 0", got)
	}
}

func TestAppendIsIdempotent(t *testing.T) {
	svc, store := newService()
	ctx := context.Background()

	events := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTackleMade, 60000, playerID(7)),
		tagged(3, core.KindTackleMade, 120000, playerID(7)),
	}

	first, err := svc.Append(ctx, clubID, teamID, matchID, events)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	second, err := svc.Append(ctx, clubID, teamID, matchID, events)
	if err != nil {
		t.Fatalf("Append() retry error = %v", err)
	}

	if got := store.storedEventCount(clubID, teamID, matchID); got != 3 {
		t.Errorf("%d events stored after a retry, want 3", got)
	}
	if got, want := second.Players[playerID(7)].Counts[core.KindTackleMade], 2; got != want {
		t.Errorf("tackles after a retry = %d, want %d", got, want)
	}
	if first.Players[playerID(7)].MinutesMs != second.Players[playerID(7)].MinutesMs {
		t.Error("a retry changed the folded minutes")
	}
}

func TestAppendReturnsTheFoldedState(t *testing.T) {
	svc, _ := newService()

	events := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTackleMade, 60000, playerID(7)),
		tagged(3, core.KindTry, 600000, playerID(11)),
	}

	state, err := svc.Append(context.Background(), clubID, teamID, matchID, events)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if got := state.Players[playerID(7)].Counts[core.KindTackleMade]; got != 1 {
		t.Errorf("p7 tackles = %d, want 1", got)
	}
	if state.Score.Us != 5 {
		t.Errorf("Score.Us = %d, want 5", state.Score.Us)
	}
	if !state.Running {
		t.Error("the clock should be running after period_started")
	}
}

func TestAppendWritesDerivedPlayerStats(t *testing.T) {
	svc, store := newService()

	events := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTackleMade, 60000, playerID(7)),
	}

	if _, err := svc.Append(context.Background(), clubID, teamID, matchID, events); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	stats := store.stats[key(clubID, teamID, matchID)]
	if len(stats) != 23 {
		t.Fatalf("%d derived rows, want one per selected player (23)", len(stats))
	}

	match := taggedMatch()
	for _, row := range stats {
		if row.TeamID != teamID || row.SeasonID != match.SeasonID || row.MatchID != matchID {
			t.Fatalf("row = %+v, want team, season and match denormalised", row)
		}
		if !row.MatchDate.Equal(match.KickoffAt) {
			t.Fatalf("MatchDate = %v, want %v", row.MatchDate, match.KickoffAt)
		}
	}

	// Jersey 7 is the seventh row because the projection is ordered by jersey.
	if got := stats[6].Counts[string(core.KindTackleMade)]; got != 1 {
		t.Errorf("p7 tackles in the derived row = %d, want 1", got)
	}
}

// Overwriting rather than merging is what makes a correction trustworthy: a re-fold
// after a void has to be able to lower a count.
func TestAppendOverwritesDerivedStats(t *testing.T) {
	svc, store := newService()
	ctx := context.Background()

	first := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTackleMade, 60000, playerID(7)),
	}
	if _, err := svc.Append(ctx, clubID, teamID, matchID, first); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	undo := []core.Event{{
		ID:       eventID(3),
		Kind:     core.KindVoid,
		ClockMs:  70000,
		Period:   1,
		DeviceID: "device-1",
		VoidsID:  eventID(2),
	}}
	state, err := svc.Append(ctx, clubID, teamID, matchID, undo)
	if err != nil {
		t.Fatalf("Append(void) error = %v", err)
	}

	if got := state.Players[playerID(7)].Counts[core.KindTackleMade]; got != 0 {
		t.Errorf("p7 tackles after the void = %d, want 0", got)
	}
	stats := store.stats[key(clubID, teamID, matchID)]
	if got := stats[6].Counts[string(core.KindTackleMade)]; got != 0 {
		t.Errorf("derived p7 tackles after the void = %d, want 0", got)
	}
}

func TestAppendPromotesMatchStatus(t *testing.T) {
	svc, store := newService()
	ctx := context.Background()

	if _, err := svc.Append(ctx, clubID, teamID, matchID, []core.Event{control(1, core.KindPeriodStarted, 0)}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	stored, _ := store.Match(ctx, clubID, teamID, matchID)
	if stored.Status != core.MatchInProgress {
		t.Errorf("Status = %q, want %q", stored.Status, core.MatchInProgress)
	}

	writes := store.putMatchCalls
	if _, err := svc.Append(ctx, clubID, teamID, matchID, []core.Event{tagged(2, core.KindCarry, 60000, playerID(12))}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	// One write saved per tap, on a free tier that counts them.
	if store.putMatchCalls != writes {
		t.Errorf("putMatchCalls = %d, want %d: an unchanged status must not be rewritten",
			store.putMatchCalls, writes)
	}

	if _, err := svc.Append(ctx, clubID, teamID, matchID, []core.Event{control(3, core.KindMatchEnded, 4800000)}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	stored, _ = store.Match(ctx, clubID, teamID, matchID)
	if stored.Status != core.MatchCompleted {
		t.Errorf("Status = %q, want %q", stored.Status, core.MatchCompleted)
	}
}

func TestAppendRejectsAnEmptyOrOversizedBatch(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	var invalid core.ValidationError
	if _, err := svc.Append(ctx, clubID, teamID, matchID, nil); !errors.As(err, &invalid) {
		t.Errorf("empty batch error = %v, want a core.ValidationError", err)
	}

	oversized := make([]core.Event, 0, MaxAppendBatch+1)
	for i := range MaxAppendBatch + 1 {
		oversized = append(oversized, tagged(i+1, core.KindTackleMade, i*1000, playerID(7)))
	}
	if _, err := svc.Append(ctx, clubID, teamID, matchID, oversized); !errors.As(err, &invalid) {
		t.Errorf("oversized batch error = %v, want a core.ValidationError", err)
	}
}

func TestAppendToUnknownMatchIsNotFound(t *testing.T) {
	svc, store := newService()

	events := []core.Event{tagged(1, core.KindTackleMade, 60000, playerID(7))}

	_, err := svc.Append(context.Background(), clubID, teamID, "no-such-match", events)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
	if store.storedEventCount(clubID, teamID, "no-such-match") != 0 {
		t.Error("events were written for a match that does not exist")
	}
}

func TestAppendAcrossClubsIsNotFound(t *testing.T) {
	svc, store := newService()

	events := []core.Event{tagged(1, core.KindTackleMade, 60000, playerID(7))}

	_, err := svc.Append(context.Background(), "club-2", teamID, matchID, events)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
	if store.storedEventCount("club-2", teamID, matchID) != 0 {
		t.Error("another club's append reached the store")
	}
}

func TestEventsSinceACursorReturnsOnlyLaterEvents(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	first := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTackleMade, 60000, playerID(7)),
	}
	if _, err := svc.Append(ctx, clubID, teamID, matchID, first); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	page, err := svc.Events(ctx, clubID, teamID, matchID, "")
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(page.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(page.Events))
	}

	// Nothing new since that cursor.
	caughtUp, err := svc.Events(ctx, clubID, teamID, matchID, page.Cursor)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(caughtUp.Events) != 0 {
		t.Errorf("len(Events) = %d, want 0 after catching up", len(caughtUp.Events))
	}
	if caughtUp.Cursor != page.Cursor {
		t.Errorf("Cursor = %q, want it to hold at %q rather than rewind", caughtUp.Cursor, page.Cursor)
	}

	if _, err := svc.Append(ctx, clubID, teamID, matchID, []core.Event{tagged(3, core.KindCarry, 120000, playerID(12))}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	next, err := svc.Events(ctx, clubID, teamID, matchID, page.Cursor)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(next.Events) != 1 || next.Events[0].Kind != core.KindCarry {
		t.Errorf("Events = %+v, want just the carry", next.Events)
	}
	if next.Cursor == page.Cursor {
		t.Error("the cursor did not advance")
	}
}

func TestEventsOnAnEmptyLogReturnsAnEmptyPage(t *testing.T) {
	svc, _ := newService()

	page, err := svc.Events(context.Background(), clubID, teamID, matchID, "")
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	// Nil marshals as `null`, which a client cannot iterate.
	if page.Events == nil {
		t.Error("Events is nil, want an empty slice")
	}
	if len(page.Events) != 0 || page.Cursor != "" {
		t.Errorf("page = %+v, want no events and no cursor", page)
	}
}

func TestEventsAcrossClubsIsNotFound(t *testing.T) {
	svc, _ := newService()

	if _, err := svc.Events(context.Background(), "club-2", teamID, matchID, ""); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}

func TestStateFoldsTheStoredLog(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	events := []core.Event{
		control(1, core.KindPeriodStarted, 0),
		tagged(2, core.KindTry, 600000, playerID(11)),
		control(3, core.KindMatchEnded, 4800000),
	}
	if _, err := svc.Append(ctx, clubID, teamID, matchID, events); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	state, err := svc.State(ctx, clubID, teamID, matchID)
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if state.Score.Us != 5 || state.Status != core.MatchCompleted {
		t.Errorf("state = %d-%d/%q, want 5-0/completed", state.Score.Us, state.Score.Them, state.Status)
	}
}

// The events are already durable when the derived write fails, so the error has to
// say which half went wrong or the log will not explain itself.
func TestAppendReportsAFailedDerivedWrite(t *testing.T) {
	svc, store := newService()
	store.statsErr = errors.New("firestore unavailable")

	events := []core.Event{tagged(1, core.KindTackleMade, 60000, playerID(7))}

	_, err := svc.Append(context.Background(), clubID, teamID, matchID, events)
	if err == nil {
		t.Fatal("Append() error = nil, want the store failure")
	}
	if !strings.Contains(err.Error(), "derived stats") {
		t.Errorf("error = %q, want it to name the derived stats write", err)
	}
	if store.storedEventCount(clubID, teamID, matchID) != 1 {
		t.Error("the events should already be durable when the derived write fails")
	}
}

func TestAppendReportsAFailedEventWrite(t *testing.T) {
	svc, store := newService()
	store.appendErr = errors.New("firestore unavailable")

	events := []core.Event{tagged(1, core.KindTackleMade, 60000, playerID(7))}

	_, err := svc.Append(context.Background(), clubID, teamID, matchID, events)
	if err == nil {
		t.Fatal("Append() error = nil, want the store failure")
	}
	if len(store.stats) != 0 {
		t.Error("derived stats were written despite the append failing")
	}
}
