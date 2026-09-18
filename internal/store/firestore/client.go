// Package firestore implements the storage interfaces declared by the service
// packages. It is the only package that knows Firestore exists.
package firestore

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
)

type Store struct {
	client *fs.Client
}

func New(ctx context.Context, projectID string) (*Store, error) {
	client, err := fs.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("firestore: new client: %w", err)
	}
	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

// clubDoc is the tenancy root. Every collection below it is scoped to one club.
func (s *Store) clubDoc(clubID string) *fs.DocumentRef {
	return s.client.Collection("clubs").Doc(clubID)
}
