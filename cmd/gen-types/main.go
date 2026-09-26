// Command gen-types narrows the TypeScript types that tygo generates.
//
// tygo renders a named Go string type as `export type Position = string`, which
// makes client-side exhaustiveness checks vacuous: Record<Position, string> would
// accept any object at all. This rewrites those aliases into real string-literal
// unions built from the Go constants, and emits the position-to-group mapping so
// the client never re-implements it.
//
// Run via `make types`, after tygo.
package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

const (
	corePath   = "web/src/types/core.ts"
	authPath   = "web/src/types/auth.ts"
	groupsPath = "web/src/types/groups.ts"
)

func main() {
	positions := strs(core.AllPositions())
	groups := strs(core.AllPositionGroups())
	statuses := []string{string(core.PlayerActive), string(core.PlayerInactive)}
	roles := strs(auth.AllRoles())

	if err := narrow(corePath, map[string][]string{
		"Position":      positions,
		"PositionGroup": groups,
		"PlayerStatus":  statuses,
		"Venue":         strs(core.AllVenues()),
		"MatchStatus":   strs(core.AllMatchStatuses()),
		"EventKind":     strs(core.AllEventKinds()),
		"EventGroup":    strs(core.AllEventGroups()),
		"Zone":          strs(core.AllZones()),
		"Possession":    strs(core.AllPossessions()),
	}); err != nil {
		fail(err)
	}

	if err := narrow(authPath, map[string][]string{
		"Role":   roles,
		"Action": strs(auth.AllActions()),
	}); err != nil {
		fail(err)
	}

	if err := writeGroups(positions); err != nil {
		fail(err)
	}

	fmt.Println("gen-types: narrowed enums and wrote", groupsPath)
}

// narrow replaces `export type X = string;` with a string-literal union.
func narrow(path string, unions map[string][]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	out := string(raw)

	names := make([]string, 0, len(unions))
	for name, values := range unions {
		alias := fmt.Sprintf("export type %s = string;", name)
		if !strings.Contains(out, alias) {
			return fmt.Errorf("%s: expected tygo to emit %q; did the Go type change?", path, alias)
		}
		out = strings.Replace(out, alias,
			fmt.Sprintf("export type %s = %s;", name, union(values)), 1)
		names = append(names, name)
	}
	slices.Sort(names)

	out = dropAnyAliases(out)
	out = recordUnionKeys(out, names)
	if err := assertNoWideAliases(path, out); err != nil {
		return err
	}
	if err := assertNoUnionIndexSignatures(path, out, names); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// recordUnionKeys rewrites the index signatures tygo emits for a Go map whose key
// is one of the narrowed types.
//
// TypeScript rejects `{ [key: Zone]: number }` outright once Zone is a union of
// literals rather than `string` — an index signature parameter cannot be a literal
// type. Partial<Record<Zone, number>> is the correct spelling, and Partial is the
// honest half of it: Fold writes only non-zero entries, so a caller has to handle a
// key that is not there.
func recordUnionKeys(src string, names []string) string {
	for _, name := range names {
		// Matched before Biome runs, so the spacing is tygo's own rather than the
		// formatted spacing the committed file ends up with.
		pattern := regexp.MustCompile(`\{\s*\[key:\s*` + regexp.QuoteMeta(name) + `\]:\s*([^{}]+?)\s*\}`)
		src = pattern.ReplaceAllString(src, "Partial<Record<"+name+", $1>>")
	}
	return src
}

// assertNoUnionIndexSignatures fails when a narrowed type is still being used as an
// index-signature key. The rewrite above depends on tygo's exact spacing, and a
// silent miss would ship a file that only `tsc` rejects, in a later job.
func assertNoUnionIndexSignatures(path, src string, names []string) error {
	var broken []string
	for _, name := range names {
		if strings.Contains(src, "[key: "+name+"]") {
			broken = append(broken, name)
		}
	}

	if len(broken) > 0 {
		return fmt.Errorf(
			"%s: %s are still index-signature keys, which TypeScript rejects for a\n"+
				"  literal union. gen-types rewrites these into Partial<Record<…>>;\n"+
				"  did tygo change how it renders a Go map?",
			path, strings.Join(broken, ", "))
	}
	return nil
}

// opaqueTypes are named Go string types that are deliberately left as `string`
// on the client because they have no fixed set of values — identifiers and the
// like. Anything not listed here must be narrowed into a union.
var opaqueTypes = map[string]bool{}

// assertNoWideAliases fails when a named string type reaches the client as
// `export type X = string`. Such an alias silently defeats exhaustiveness
// checks: Record<X, string> would accept any object at all. Adding an enum in
// Go and forgetting to register it above should break the build, not quietly
// produce a type that checks nothing.
func assertNoWideAliases(path, src string) error {
	var wide []string
	for line := range strings.Lines(src) {
		trimmed := strings.TrimSpace(line)
		name, ok := strings.CutPrefix(trimmed, "export type ")
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, " = string;")
		if !ok || opaqueTypes[name] {
			continue
		}
		wide = append(wide, name)
	}

	if len(wide) > 0 {
		return fmt.Errorf(
			"%s: %s reached the client as `= string`.\n"+
				"  Register each in cmd/gen-types (with an All…() accessor in Go) so it becomes a\n"+
				"  string-literal union, or add it to opaqueTypes if it genuinely has no fixed values.",
			path, strings.Join(wide, ", "))
	}
	return nil
}

// dropAnyAliases removes the `export type X = any;` lines tygo emits for Go
// interfaces. They carry no information for the client and trip Biome's
// noExplicitAny rule.
func dropAnyAliases(src string) string {
	var kept []string
	for line := range strings.Lines(src) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export type ") && strings.HasSuffix(trimmed, "= any;") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

// writeGroups emits the position-to-group lookup, so the client derives a
// player's comparison group from the same table Go uses.
func writeGroups(positions []string) error {
	var b strings.Builder
	b.WriteString("// Code generated by cmd/gen-types. DO NOT EDIT.\n\n")
	b.WriteString(`import type { Position, PositionGroup } from "./core";` + "\n\n")

	b.WriteString("export const POSITIONS: readonly Position[] = [\n")
	for _, p := range positions {
		fmt.Fprintf(&b, "\t%q,\n", p)
	}
	b.WriteString("];\n\n")

	b.WriteString("export const POSITION_GROUPS: readonly PositionGroup[] = [\n")
	for _, g := range core.AllPositionGroups() {
		fmt.Fprintf(&b, "\t%q,\n", string(g))
	}
	b.WriteString("];\n\n")

	b.WriteString("export const GROUP_OF_POSITION: Record<Position, PositionGroup> = {\n")
	for _, p := range core.AllPositions() {
		fmt.Fprintf(&b, "\t%q: %q,\n", string(p), string(p.Group()))
	}
	b.WriteString("};\n")

	return os.WriteFile(groupsPath, []byte(b.String()), 0o644)
}

func union(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(quoted, " | ")
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-types:", err)
	os.Exit(1)
}
