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

func (s *Store) teamsCol(clubID string) *fs.CollectionRef {
	return s.clubDoc(clubID).Collection("teams")
}

func (s *Store) Team(ctx context.Context, clubID, teamID string) (core.Team, error) {
	snap, err := s.teamsCol(clubID).Doc(teamID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return core.Team{}, core.ErrNotFound
	}
	if err != nil {
		return core.Team{}, fmt.Errorf("firestore: get team %q: %w", teamID, err)
	}
	return teamFromSnap(snap, clubID)
}

func (s *Store) Teams(ctx context.Context, clubID string) ([]core.Team, error) {
	iter := s.teamsCol(clubID).OrderBy("name", fs.Asc).Documents(ctx)
	defer iter.Stop()

	var out []core.Team
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore: list teams: %w", err)
		}
		t, err := teamFromSnap(snap, clubID)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
}

func (s *Store) PutTeam(ctx context.Context, clubID string, t core.Team) error {
	if _, err := s.teamsCol(clubID).Doc(t.ID).Set(ctx, t); err != nil {
		return fmt.Errorf("firestore: put team %q: %w", t.ID, err)
	}
	return nil
}

// DeleteTeam removes the team and everything nested beneath it.
//
// Firestore does not cascade: deleting a document leaves its subcollections in
// place as unreachable orphans that still cost storage and still surface in
// collection group queries. The Go client has no RecursiveDelete helper either,
// so the descendants are walked explicitly — a squad's fixtures, their events
// and their derived stats all go with it.
func (s *Store) DeleteTeam(ctx context.Context, clubID, teamID string) error {
	writer := s.client.BulkWriter(ctx)

	jobs, err := s.deleteTree(ctx, writer, s.teamsCol(clubID).Doc(teamID), nil)
	if err != nil {
		return fmt.Errorf("firestore: delete team %q: %w", teamID, err)
	}
	writer.End()

	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return fmt.Errorf("firestore: delete team %q: %w", teamID, err)
		}
	}
	return nil
}

// deleteTree queues deletes for doc and every descendant, depth first so a
// parent is never removed before the children that hang off it.
func (s *Store) deleteTree(
	ctx context.Context,
	writer *fs.BulkWriter,
	doc *fs.DocumentRef,
	jobs []*fs.BulkWriterJob,
) ([]*fs.BulkWriterJob, error) {
	cols := doc.Collections(ctx)
	for {
		col, err := cols.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing subcollections of %q: %w", doc.Path, err)
		}

		// DocumentRefs also yields "missing" documents that exist only as
		// parents of a subcollection, which is exactly what must be walked.
		refs := col.DocumentRefs(ctx)
		for {
			child, err := refs.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("listing documents of %q: %w", col.Path, err)
			}
			if jobs, err = s.deleteTree(ctx, writer, child, jobs); err != nil {
				return nil, err
			}
		}
	}

	job, err := writer.Delete(doc)
	if err != nil {
		return nil, fmt.Errorf("queueing delete of %q: %w", doc.Path, err)
	}
	return append(jobs, job), nil
}

func teamFromSnap(snap *fs.DocumentSnapshot, clubID string) (core.Team, error) {
	var t core.Team
	if err := snap.DataTo(&t); err != nil {
		return core.Team{}, fmt.Errorf("firestore: decode team %q: %w", snap.Ref.ID, err)
	}
	t.ID = snap.Ref.ID
	t.ClubID = clubID
	return t, nil
}
