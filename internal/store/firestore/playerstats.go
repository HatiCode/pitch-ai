package firestore

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"

	"pitch-ai/internal/core"
)

// playerStatsCol holds one document per selected player, keyed by player ID.
//
// The collection name is load-bearing: a season trend is a collection-group query
// over "playerStats" across every match, so renaming it here would silently return
// nothing there rather than fail.
func (s *Store) playerStatsCol(clubID, teamID, matchID string) *fs.CollectionRef {
	return s.matchesCol(clubID, teamID).Doc(matchID).Collection("playerStats")
}

// PutPlayerStats overwrites the derived documents for a match.
//
// Set rather than a merge, because these are a projection of the log and not a
// running total: voiding a mis-tapped tackle has to lower the count, and a merge
// would leave the higher figure in place with nothing able to correct it.
func (s *Store) PutPlayerStats(ctx context.Context, clubID, teamID, matchID string, stats []core.PlayerMatchStats) error {
	if len(stats) == 0 {
		return nil
	}

	col := s.playerStatsCol(clubID, teamID, matchID)
	writer := s.client.BulkWriter(ctx)
	jobs := make([]*fs.BulkWriterJob, 0, len(stats))

	for _, player := range stats {
		job, err := writer.Set(col.Doc(player.PlayerID), player)
		if err != nil {
			return fmt.Errorf("firestore: queueing stats for player %q: %w", player.PlayerID, err)
		}
		jobs = append(jobs, job)
	}
	writer.End()

	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return fmt.Errorf("firestore: put player stats for match %q: %w", matchID, err)
		}
	}
	return nil
}
