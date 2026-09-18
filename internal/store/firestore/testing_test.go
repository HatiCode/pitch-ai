package firestore

import (
	"context"
	"os"
	"testing"
)

// newTestStore connects to the Firestore emulator, skipping the test when it is
// not running so `go test ./...` stays green without external services.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; run `make emulator` and use `make test-store`")
	}

	store, err := New(context.Background(), "pitch-ai-test")
	if err != nil {
		t.Fatalf("connecting to emulator: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
