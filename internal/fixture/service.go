// Package fixture holds the match-scheduling use-cases. "Fixture" is the rugby
// sense of the word: a scheduled match.
package fixture

import (
	"context"
	"fmt"
	"slices"

	"pitch-ai/internal/core"
)

type MatchReader interface {
	Match(ctx context.Context, clubID, teamID, matchID string) (core.Match, error)
	Matches(ctx context.Context, clubID, teamID string) ([]core.Match, error)
}

type MatchWriter interface {
	PutMatch(ctx context.Context, clubID, teamID string, m core.Match) error
}

// SquadReader supplies the players a lineup may be selected from.
type SquadReader interface {
	Players(ctx context.Context, clubID string) ([]core.Player, error)
}

type Service struct {
	reader MatchReader
	writer MatchWriter
	squad  SquadReader
	newID  func() string
}

func NewService(r MatchReader, w MatchWriter, s SquadReader, newID func() string) *Service {
	return &Service{reader: r, writer: w, squad: s, newID: newID}
}

// Schedule creates a fixture. Identity always comes from the caller's context,
// never from the request body.
func (s *Service) Schedule(ctx context.Context, clubID, teamID string, m core.Match) (core.Match, error) {
	m.ID = s.newID()
	m.ClubID = clubID
	m.TeamID = teamID
	if m.Status == "" {
		m.Status = core.MatchScheduled
	}

	if err := m.Validate(); err != nil {
		return core.Match{}, err
	}
	if err := s.writer.PutMatch(ctx, clubID, teamID, m); err != nil {
		return core.Match{}, fmt.Errorf("fixture: schedule match: %w", err)
	}
	return m, nil
}

func (s *Service) Get(ctx context.Context, clubID, teamID, matchID string) (core.Match, error) {
	return s.reader.Match(ctx, clubID, teamID, matchID)
}

func (s *Service) List(ctx context.Context, clubID, teamID string) ([]core.Match, error) {
	return s.reader.Matches(ctx, clubID, teamID)
}

// SetLineup replaces a match's selection. The lineup is validated against the
// squad as it stands now, so a player who has left the squad cannot be selected
// even if they were eligible when the fixture was created.
func (s *Service) SetLineup(ctx context.Context, clubID, teamID, matchID string, lineup core.Lineup) (core.Match, error) {
	match, err := s.reader.Match(ctx, clubID, teamID, matchID)
	if err != nil {
		return core.Match{}, err
	}

	eligible, err := s.eligiblePlayers(ctx, clubID, teamID)
	if err != nil {
		return core.Match{}, err
	}
	if err := lineup.Validate(eligible); err != nil {
		return core.Match{}, err
	}

	match.Lineup = lineup
	if err := s.writer.PutMatch(ctx, clubID, teamID, match); err != nil {
		return core.Match{}, fmt.Errorf("fixture: set lineup on %q: %w", matchID, err)
	}
	return match, nil
}

// eligiblePlayers is the set a lineup may draw from: active players who are
// members of this squad.
func (s *Service) eligiblePlayers(ctx context.Context, clubID, teamID string) (map[string]bool, error) {
	players, err := s.squad.Players(ctx, clubID)
	if err != nil {
		return nil, fmt.Errorf("fixture: load squad: %w", err)
	}

	eligible := make(map[string]bool, len(players))
	for _, p := range players {
		if p.Status == core.PlayerActive && slices.Contains(p.TeamIDs, teamID) {
			eligible[p.ID] = true
		}
	}
	return eligible, nil
}
