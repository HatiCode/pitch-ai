// Package squad holds the player-management use-cases. It declares the storage
// interfaces it needs and knows nothing about Firestore or HTTP.
package squad

import (
	"context"
	"fmt"

	"pitch-ai/internal/core"
)

type PlayerReader interface {
	Player(ctx context.Context, clubID, playerID string) (core.Player, error)
	Players(ctx context.Context, clubID string) ([]core.Player, error)
}

type PlayerWriter interface {
	PutPlayer(ctx context.Context, clubID string, p core.Player) error
	DeletePlayer(ctx context.Context, clubID, playerID string) error
}

type Service struct {
	reader PlayerReader
	writer PlayerWriter
	newID  func() string
}

func NewService(r PlayerReader, w PlayerWriter, newID func() string) *Service {
	return &Service{reader: r, writer: w, newID: newID}
}

// Add stores a new player. The caller's clubID always wins over anything in p.
func (s *Service) Add(ctx context.Context, clubID string, p core.Player) (core.Player, error) {
	p.ID = s.newID()
	p.ClubID = clubID
	if p.Status == "" {
		p.Status = core.PlayerActive
	}

	if err := p.Validate(); err != nil {
		return core.Player{}, err
	}
	if err := s.writer.PutPlayer(ctx, clubID, p); err != nil {
		return core.Player{}, fmt.Errorf("squad: add player: %w", err)
	}
	return p, nil
}

// Update replaces a player's mutable fields. Identity comes from the path, never
// from the request body. The existence check is club-scoped, so a player in
// another club is indistinguishable from one that does not exist.
func (s *Service) Update(ctx context.Context, clubID, playerID string, p core.Player) (core.Player, error) {
	if _, err := s.reader.Player(ctx, clubID, playerID); err != nil {
		return core.Player{}, err
	}

	p.ID = playerID
	p.ClubID = clubID
	if p.Status == "" {
		p.Status = core.PlayerActive
	}

	if err := p.Validate(); err != nil {
		return core.Player{}, err
	}
	if err := s.writer.PutPlayer(ctx, clubID, p); err != nil {
		return core.Player{}, fmt.Errorf("squad: update player %q: %w", playerID, err)
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, clubID, playerID string) (core.Player, error) {
	return s.reader.Player(ctx, clubID, playerID)
}

func (s *Service) List(ctx context.Context, clubID string) ([]core.Player, error) {
	return s.reader.Players(ctx, clubID)
}

func (s *Service) Remove(ctx context.Context, clubID, playerID string) error {
	return s.writer.DeletePlayer(ctx, clubID, playerID)
}
