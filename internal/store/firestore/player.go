package firestore

import (
	"context"
	"errors"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pitch-ai/internal/core"
)

func (s *Store) playersCol(clubID string) *fs.CollectionRef {
	return s.clubDoc(clubID).Collection("players")
}

func (s *Store) Player(ctx context.Context, clubID, playerID string) (core.Player, error) {
	snap, err := s.playersCol(clubID).Doc(playerID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return core.Player{}, core.ErrNotFound
	}
	if err != nil {
		return core.Player{}, fmt.Errorf("firestore: get player %q: %w", playerID, err)
	}
	return playerFromSnap(snap, clubID)
}

func (s *Store) Players(ctx context.Context, clubID string) ([]core.Player, error) {
	iter := s.playersCol(clubID).OrderBy("lastName", fs.Asc).Documents(ctx)
	defer iter.Stop()

	var out []core.Player
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore: list players: %w", err)
		}
		p, err := playerFromSnap(snap, clubID)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
}

func (s *Store) PutPlayer(ctx context.Context, clubID string, p core.Player) error {
	if _, err := s.playersCol(clubID).Doc(p.ID).Set(ctx, p); err != nil {
		return fmt.Errorf("firestore: put player %q: %w", p.ID, err)
	}
	return nil
}

func (s *Store) DeletePlayer(ctx context.Context, clubID, playerID string) error {
	if _, err := s.playersCol(clubID).Doc(playerID).Delete(ctx); err != nil {
		return fmt.Errorf("firestore: delete player %q: %w", playerID, err)
	}
	return nil
}

// playerFromSnap decodes a document and restores the identity fields, which are
// carried by the document path rather than stored in the document body.
func playerFromSnap(snap *fs.DocumentSnapshot, clubID string) (core.Player, error) {
	var p core.Player
	if err := snap.DataTo(&p); err != nil {
		return core.Player{}, fmt.Errorf("firestore: decode player %q: %w", snap.Ref.ID, err)
	}
	p.ID = snap.Ref.ID
	p.ClubID = clubID
	return p, nil
}
