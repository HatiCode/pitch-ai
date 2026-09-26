package match

import (
	"context"
	"slices"
	"strconv"

	"pitch-ai/internal/core"
)

// memStore satisfies every interface this package declares. Keying by club and team
// is what lets the tests prove tenant isolation, and keying events by their own ID
// is what lets them prove idempotency.
type memStore struct {
	matches map[string]core.Match
	events  map[string]map[string]storedEvent
	stats   map[string][]core.PlayerMatchStats

	// seq stands in for the arrival order a real cursor walks, which Firestore
	// gets from a server timestamp.
	seq int

	putMatchCalls int
	appendErr     error
	statsErr      error
}

type storedEvent struct {
	event core.Event
	seq   int
}

func newMemStore() *memStore {
	return &memStore{
		matches: map[string]core.Match{},
		events:  map[string]map[string]storedEvent{},
		stats:   map[string][]core.PlayerMatchStats{},
	}
}

func key(clubID, teamID, matchID string) string { return clubID + "/" + teamID + "/" + matchID }

func (m *memStore) Match(_ context.Context, clubID, teamID, matchID string) (core.Match, error) {
	match, ok := m.matches[key(clubID, teamID, matchID)]
	if !ok {
		return core.Match{}, core.ErrNotFound
	}
	return match, nil
}

func (m *memStore) PutMatch(_ context.Context, clubID, teamID string, match core.Match) error {
	m.putMatchCalls++
	m.matches[key(clubID, teamID, match.ID)] = match
	return nil
}

func (m *memStore) AppendEvents(_ context.Context, clubID, teamID, matchID string, events []core.Event) error {
	if m.appendErr != nil {
		return m.appendErr
	}

	k := key(clubID, teamID, matchID)
	if m.events[k] == nil {
		m.events[k] = map[string]storedEvent{}
	}
	for _, e := range events {
		m.seq++
		m.events[k][e.ID] = storedEvent{event: e, seq: m.seq}
	}
	return nil
}

func (m *memStore) Events(_ context.Context, clubID, teamID, matchID, since string) ([]core.Event, string, error) {
	after := 0
	if since != "" {
		n, err := strconv.Atoi(since)
		if err != nil {
			return nil, "", core.ValidationError{Field: "since", Reason: "malformed cursor"}
		}
		after = n
	}

	var stored []storedEvent
	for _, s := range m.events[key(clubID, teamID, matchID)] {
		if s.seq > after {
			stored = append(stored, s)
		}
	}
	slices.SortFunc(stored, func(a, b storedEvent) int { return a.seq - b.seq })

	out := make([]core.Event, 0, len(stored))
	cursor := since
	for _, s := range stored {
		out = append(out, s.event)
		cursor = strconv.Itoa(s.seq)
	}
	return out, cursor, nil
}

func (m *memStore) PutPlayerStats(_ context.Context, clubID, teamID, matchID string, stats []core.PlayerMatchStats) error {
	if m.statsErr != nil {
		return m.statsErr
	}
	m.stats[key(clubID, teamID, matchID)] = stats
	return nil
}

func (m *memStore) storedEventCount(clubID, teamID, matchID string) int {
	return len(m.events[key(clubID, teamID, matchID)])
}
