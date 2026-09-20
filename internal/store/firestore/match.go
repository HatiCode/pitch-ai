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

func (s *Store) matchesCol(clubID, teamID string) *fs.CollectionRef {
	return s.teamsCol(clubID).Doc(teamID).Collection("matches")
}

func (s *Store) Match(ctx context.Context, clubID, teamID, matchID string) (core.Match, error) {
	snap, err := s.matchesCol(clubID, teamID).Doc(matchID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return core.Match{}, core.ErrNotFound
	}
	if err != nil {
		return core.Match{}, fmt.Errorf("firestore: get match %q: %w", matchID, err)
	}
	return matchFromSnap(snap, clubID, teamID)
}

// Matches lists a squad's fixtures, most recent kick-off first.
func (s *Store) Matches(ctx context.Context, clubID, teamID string) ([]core.Match, error) {
	iter := s.matchesCol(clubID, teamID).OrderBy("kickoffAt", fs.Desc).Documents(ctx)
	defer iter.Stop()

	var out []core.Match
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore: list matches: %w", err)
		}
		m, err := matchFromSnap(snap, clubID, teamID)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
}

func (s *Store) PutMatch(ctx context.Context, clubID, teamID string, m core.Match) error {
	if _, err := s.matchesCol(clubID, teamID).Doc(m.ID).Set(ctx, m); err != nil {
		return fmt.Errorf("firestore: put match %q: %w", m.ID, err)
	}
	return nil
}

func matchFromSnap(snap *fs.DocumentSnapshot, clubID, teamID string) (core.Match, error) {
	var m core.Match
	if err := snap.DataTo(&m); err != nil {
		return core.Match{}, fmt.Errorf("firestore: decode match %q: %w", snap.Ref.ID, err)
	}
	m.ID = snap.Ref.ID
	m.ClubID = clubID
	m.TeamID = teamID
	return m, nil
}
