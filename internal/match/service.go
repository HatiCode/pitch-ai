// Package match holds the live-tagging use-cases: appending to a match's event log
// and reading the state that log implies.
package match

import (
	"context"
	"fmt"
	"strconv"

	"pitch-ai/internal/core"
)

// MaxAppendBatch caps one append. The client drains its outbox well below this; the
// cap exists so a malformed or hostile request cannot ask the server to fold an
// unbounded batch in one transaction.
const MaxAppendBatch = 500

type MatchReader interface {
	Match(ctx context.Context, clubID, teamID, matchID string) (core.Match, error)
}

type MatchWriter interface {
	PutMatch(ctx context.Context, clubID, teamID string, m core.Match) error
}

type EventStore interface {
	AppendEvents(ctx context.Context, clubID, teamID, matchID string, events []core.Event) error
}

type EventReader interface {
	Events(ctx context.Context, clubID, teamID, matchID, since string) ([]core.Event, string, error)
}

type StatsWriter interface {
	PutPlayerStats(ctx context.Context, clubID, teamID, matchID string, stats []core.PlayerMatchStats) error
}

// Page is one batch of events and the cursor a client resumes from.
type Page struct {
	Events []core.Event `json:"events"`
	Cursor string       `json:"cursor"`
}

type Service struct {
	matches MatchReader
	writer  MatchWriter
	events  EventStore
	log     EventReader
	stats   StatsWriter
}

func NewService(matches MatchReader, writer MatchWriter, events EventStore, log EventReader, stats StatsWriter) *Service {
	return &Service{matches: matches, writer: writer, events: events, log: log, stats: stats}
}

// Append validates a batch against the match's selected squad, appends it, and
// recomputes the derived per-player documents from the whole log.
//
// The endpoint behind this is idempotent: events are stored keyed by their own ID,
// so a retry after a flaky response overwrites identical data. That is what lets a
// client on a sideline retry blindly rather than ask whether a tap arrived.
func (s *Service) Append(ctx context.Context, clubID, teamID, matchID string, events []core.Event) (core.MatchState, error) {
	if len(events) == 0 {
		return core.MatchState{}, core.ValidationError{Field: "events", Reason: "must not be empty"}
	}
	if len(events) > MaxAppendBatch {
		return core.MatchState{}, core.ValidationError{
			Field:  "events",
			Reason: "at most " + strconv.Itoa(MaxAppendBatch) + " events in one batch",
		}
	}

	match, err := s.matches.Match(ctx, clubID, teamID, matchID)
	if err != nil {
		return core.MatchState{}, err
	}

	// The whole batch is validated before any of it is written. A half-applied
	// batch is indistinguishable from a partial sync, and would leave the client
	// retrying events the server has already rejected.
	squad := squadOf(match.Lineup)
	for _, e := range events {
		if err := e.Validate(squad); err != nil {
			return core.MatchState{}, err
		}
	}

	if err := s.events.AppendEvents(ctx, clubID, teamID, matchID, events); err != nil {
		return core.MatchState{}, fmt.Errorf("match: append %d events to %q: %w", len(events), matchID, err)
	}
	return s.recompute(ctx, clubID, teamID, match)
}

// Events returns the events appended after the given cursor, oldest first, together
// with the cursor to resume from next time. An empty cursor reads the whole log.
func (s *Service) Events(ctx context.Context, clubID, teamID, matchID, since string) (Page, error) {
	// Reading the match is what enforces tenancy: an event log is never reachable
	// without its match existing in this club and team.
	if _, err := s.matches.Match(ctx, clubID, teamID, matchID); err != nil {
		return Page{}, err
	}

	events, cursor, err := s.log.Events(ctx, clubID, teamID, matchID, since)
	if err != nil {
		return Page{}, fmt.Errorf("match: read events of %q: %w", matchID, err)
	}
	if events == nil {
		// Nil marshals as `null`, which a client cannot iterate.
		events = []core.Event{}
	}
	return Page{Events: events, Cursor: cursor}, nil
}

// State folds the stored log, for a caller that has no local copy to fold itself.
func (s *Service) State(ctx context.Context, clubID, teamID, matchID string) (core.MatchState, error) {
	match, log, err := s.matchAndLog(ctx, clubID, teamID, matchID)
	if err != nil {
		return core.MatchState{}, err
	}
	return core.Fold(match, log), nil
}

// recompute re-folds the whole log and overwrites the derived documents.
//
// Deliberately total rather than incremental. A match is a few hundred events, so
// re-folding costs microseconds, and a bug in the stats engine is then fixed by
// deploying and re-appending rather than by migrating totals that have already
// drifted. Overwriting rather than merging is also what lets a void lower a count.
func (s *Service) recompute(ctx context.Context, clubID, teamID string, match core.Match) (core.MatchState, error) {
	log, _, err := s.log.Events(ctx, clubID, teamID, match.ID, "")
	if err != nil {
		return core.MatchState{}, fmt.Errorf("match: read log of %q: %w", match.ID, err)
	}

	state := core.Fold(match, log)
	if err := s.stats.PutPlayerStats(ctx, clubID, teamID, match.ID, state.PlayerMatchStats(match)); err != nil {
		return core.MatchState{}, fmt.Errorf("match: write derived stats for %q: %w", match.ID, err)
	}

	// Only when the log has actually moved the match on: one write saved per tap,
	// on a free tier that counts them.
	if state.Status != match.Status {
		match.Status = state.Status
		if err := s.writer.PutMatch(ctx, clubID, teamID, match); err != nil {
			return core.MatchState{}, fmt.Errorf("match: update status of %q: %w", match.ID, err)
		}
	}
	return state, nil
}

func (s *Service) matchAndLog(ctx context.Context, clubID, teamID, matchID string) (core.Match, []core.Event, error) {
	match, err := s.matches.Match(ctx, clubID, teamID, matchID)
	if err != nil {
		return core.Match{}, nil, err
	}

	log, _, err := s.log.Events(ctx, clubID, teamID, matchID, "")
	if err != nil {
		return core.Match{}, nil, fmt.Errorf("match: read log of %q: %w", matchID, err)
	}
	return match, log, nil
}

// squadOf is the set an event may name: everyone selected for this match, starters
// and replacements alike.
func squadOf(lineup core.Lineup) map[string]bool {
	squad := make(map[string]bool, len(lineup.Starters)+len(lineup.Bench))
	for _, slot := range lineup.Starters {
		squad[slot.PlayerID] = true
	}
	for _, slot := range lineup.Bench {
		squad[slot.PlayerID] = true
	}
	return squad
}
