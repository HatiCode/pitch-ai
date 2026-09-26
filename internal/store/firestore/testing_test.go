package firestore

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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

// uniqueClubID keeps one run's documents out of the next one's way. The emulator
// holds its data for as long as the container lives, and a test that appends to a
// log rather than overwriting a fixed document would otherwise read back
// everything the previous run left behind.
func uniqueClubID(t *testing.T) string {
	t.Helper()
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
