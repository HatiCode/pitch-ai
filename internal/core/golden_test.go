package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// goldenDir holds the fixtures the TypeScript fold also runs. See its README for
// the format and for why `expected` is reviewed rather than blessed.
const goldenDir = "../../testdata/golden"

type goldenFixture struct {
	Name     string          `json:"name"`
	Match    Match           `json:"match"`
	Events   []Event         `json:"events"`
	Expected json.RawMessage `json:"expected"`
}

func TestGoldenFixtures(t *testing.T) {
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("reading %s: %v", goldenDir, err)
	}

	fixtures := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		fixtures++

		t.Run(strings.TrimSuffix(entry.Name(), ".json"), func(t *testing.T) {
			fixture := readGoldenFixture(t, filepath.Join(goldenDir, entry.Name()))

			got := canonical(t, mustMarshal(t, Fold(fixture.Match, fixture.Events)))
			want := canonical(t, fixture.Expected)

			if !reflect.DeepEqual(got, want) {
				// A diff of two forty-line objects is the only way this failure is
				// debuggable, so both sides are printed in full.
				t.Errorf("%s\n\ngot:\n%s\n\nwant:\n%s",
					fixture.Name, reindent(t, got), reindent(t, want))
			}
		})
	}

	// A golden suite that silently runs zero cases is worse than no suite at all.
	if fixtures == 0 {
		t.Fatalf("no .json fixtures found in %s", goldenDir)
	}
}

func readGoldenFixture(t *testing.T, path string) goldenFixture {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var fixture goldenFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	if len(fixture.Expected) == 0 {
		t.Fatalf("%s has no expected state", path)
	}
	return fixture
}

// canonical decodes JSON into plain maps and slices so that key order and
// whitespace cannot affect the comparison — only the values can.
func canonical(t *testing.T, raw []byte) any {
	t.Helper()

	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("canonicalising %s: %v", raw, err)
	}
	return out
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling %T: %v", v, err)
	}
	return raw
}

func reindent(t *testing.T, v any) string {
	t.Helper()

	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("indenting %T: %v", v, err)
	}
	return string(raw)
}
