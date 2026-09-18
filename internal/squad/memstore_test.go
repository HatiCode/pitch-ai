package squad

import (
	"context"
	"sort"

	"pitch-ai/internal/core"
)

// memStore is an in-memory PlayerReader + PlayerWriter for service tests.
// Keying by club is what lets the tests prove tenant isolation.
type memStore struct {
	players map[string]map[string]core.Player // clubID -> playerID -> player
}

func newMemStore() *memStore {
	return &memStore{players: map[string]map[string]core.Player{}}
}

func (m *memStore) Player(_ context.Context, clubID, playerID string) (core.Player, error) {
	p, ok := m.players[clubID][playerID]
	if !ok {
		return core.Player{}, core.ErrNotFound
	}
	return p, nil
}

func (m *memStore) Players(_ context.Context, clubID string) ([]core.Player, error) {
	out := make([]core.Player, 0, len(m.players[clubID]))
	for _, p := range m.players[clubID] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastName < out[j].LastName })
	return out, nil
}

func (m *memStore) PutPlayer(_ context.Context, clubID string, p core.Player) error {
	if m.players[clubID] == nil {
		m.players[clubID] = map[string]core.Player{}
	}
	m.players[clubID][p.ID] = p
	return nil
}

func (m *memStore) DeletePlayer(_ context.Context, clubID, playerID string) error {
	if _, ok := m.players[clubID][playerID]; !ok {
		return core.ErrNotFound
	}
	delete(m.players[clubID], playerID)
	return nil
}
