package squad

import (
	"context"
	"fmt"
	"slices"

	"pitch-ai/internal/core"
)

type TeamReader interface {
	Team(ctx context.Context, clubID, teamID string) (core.Team, error)
	Teams(ctx context.Context, clubID string) ([]core.Team, error)
}

type TeamWriter interface {
	PutTeam(ctx context.Context, clubID string, t core.Team) error
	// DeleteTeam must remove the team document and everything beneath it —
	// fixtures, events, derived stats — not just the document itself.
	DeleteTeam(ctx context.Context, clubID, teamID string) error
}

// PlayerDetacher lets Delete drop a disbanded squad from its members without
// the team service owning player storage.
type PlayerDetacher interface {
	Players(ctx context.Context, clubID string) ([]core.Player, error)
	PutPlayer(ctx context.Context, clubID string, p core.Player) error
}

// TeamService manages the squads a club fields. Players are not owned by a
// team — see Service for those — so creating or renaming a squad never touches
// player records.
type TeamService struct {
	reader  TeamReader
	writer  TeamWriter
	players PlayerDetacher
	newID   func() string
}

func NewTeamService(r TeamReader, w TeamWriter, p PlayerDetacher, newID func() string) *TeamService {
	return &TeamService{reader: r, writer: w, players: p, newID: newID}
}

func (s *TeamService) Create(ctx context.Context, clubID string, t core.Team) (core.Team, error) {
	t.ID = s.newID()
	t.ClubID = clubID
	t.Active = true

	if err := t.Validate(); err != nil {
		return core.Team{}, err
	}
	if err := s.writer.PutTeam(ctx, clubID, t); err != nil {
		return core.Team{}, fmt.Errorf("squad: create team: %w", err)
	}
	return t, nil
}

// Update replaces a team's mutable fields. The club-scoped existence check
// means a team in another club is reported as not found rather than forbidden.
func (s *TeamService) Update(ctx context.Context, clubID, teamID string, t core.Team) (core.Team, error) {
	if _, err := s.reader.Team(ctx, clubID, teamID); err != nil {
		return core.Team{}, err
	}

	t.ID = teamID
	t.ClubID = clubID

	if err := t.Validate(); err != nil {
		return core.Team{}, err
	}
	if err := s.writer.PutTeam(ctx, clubID, t); err != nil {
		return core.Team{}, fmt.Errorf("squad: update team %q: %w", teamID, err)
	}
	return t, nil
}

// Delete disbands a squad: its fixtures and everything derived from them go,
// but its players do not. Players belong to the club, so each member simply
// stops being in this squad — including a member who was in no other, who stays
// on the club's books unassigned rather than being destroyed.
//
// Players are detached before the team is removed. If the process dies midway,
// the result is a squad with no members rather than players still pointing at a
// squad that no longer exists.
func (s *TeamService) Delete(ctx context.Context, clubID, teamID string) error {
	if _, err := s.reader.Team(ctx, clubID, teamID); err != nil {
		return err
	}

	players, err := s.players.Players(ctx, clubID)
	if err != nil {
		return fmt.Errorf("squad: load players before deleting team %q: %w", teamID, err)
	}

	for _, p := range players {
		if !slices.Contains(p.TeamIDs, teamID) {
			continue
		}
		p.TeamIDs = slices.DeleteFunc(slices.Clone(p.TeamIDs), func(id string) bool {
			return id == teamID
		})
		if err := s.players.PutPlayer(ctx, clubID, p); err != nil {
			return fmt.Errorf("squad: detach player %q from team %q: %w", p.ID, teamID, err)
		}
	}

	if err := s.writer.DeleteTeam(ctx, clubID, teamID); err != nil {
		return fmt.Errorf("squad: delete team %q: %w", teamID, err)
	}
	return nil
}

func (s *TeamService) Get(ctx context.Context, clubID, teamID string) (core.Team, error) {
	return s.reader.Team(ctx, clubID, teamID)
}

func (s *TeamService) List(ctx context.Context, clubID string) ([]core.Team, error) {
	return s.reader.Teams(ctx, clubID)
}
