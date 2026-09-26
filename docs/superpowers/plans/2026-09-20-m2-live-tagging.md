# M2: Live Tagging — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A coach on an iPad can tag a full match with no network, see live counts and minutes while doing it, correct mis-taps, and have every event reach the server once signal returns — with the same numbers computed on the device and on the server.

**Architecture:** Every tap appends an immutable event to IndexedDB; the UI renders from local state. A background outbox drains events to an idempotent endpoint. Match state — score, clock, territory, minutes, per-player counts — is a pure fold over the log, implemented twice (Go and TypeScript) and pinned to shared golden fixtures so the two cannot drift.

**Tech Stack:** Additions to the M0/M1 stack — Dexie 4 + `dexie-react-hooks` (IndexedDB), `vite-plugin-pwa` (service worker and manifest), `fake-indexeddb` (Vitest), `@playwright/test` (one offline end-to-end test). No new Go dependencies: `github.com/google/uuid` is already present and provides UUIDv7.

**Spec:** `docs/superpowers/specs/2026-09-17-rugby-performance-tracker-design.md` (sections 4–7, 12)

**Depends on:** `docs/superpowers/plans/2026-09-17-m0-m1-skeleton-and-squad.md` — `core.ValidationError`, `core.ErrNotFound`, `Deps.protected`, `teamFromPath`, `Store.clubDoc`, the `make types` pipeline, and `core.Match.Lineup`.

---

## Global Constraints

Carried forward from M0/M1 and still binding:

- Go module path `pitch-ai`. Go 1.27, TypeScript `strict: true`, Node 26.
- **`clubID` is always read from the caller's auth context, never from a request body or query parameter.**
- `internal/core` imports only the Go standard library.
- Interfaces are declared by the package that consumes them, 1–3 methods each.
- No state management library in the web app. IndexedDB via Dexie is the state; TanStack Query only for server-only reads.
- `min-instances=0`, free tier, no new paid service.
- **Alex runs `git commit` himself.** Every task ends by staging with `git add` and giving the commit message to use. Do not run `git commit`.

New for M2:

- **Event payloads carry strings only.** `map[string]any` round-trips through JSON, where every number becomes a `float64`; a payload of strings compares identically in Go, in TypeScript, and in a golden fixture.
- **The fold is pure and total.** No wall clock, no randomness, no I/O, no error return. Given the same log in any order it returns the same state.
- **The fold knows semantics; the catalogue knows presentation.** Which kinds are cards, subs, toggles or scores, and how many points a score is worth, is behaviour and lives in both folds. Labels, tab grouping and button order are configuration served to the client.

---

## Decisions taken here

Four points where this plan resolves something the spec left open or underspecified. A reviewer should read these before the tasks.

**1. Scoring events are added to the catalogue.** Spec section 6 lists only `try`, but section 5 and section 7 both require a score. Tries alone cannot produce one. Added: `conversion_made`, `conversion_missed`, `penalty_goal`, `drop_goal` (ours, player-attributed) and `opposition_try`, `opposition_conversion`, `opposition_penalty`, `opposition_drop` (team-level). They form a `score` catalogue group rendered as a compact strip beside the scoreboard, **not** a fourth tab — spec section 7 is explicit that the PLAY tab is at its button ceiling and that the remedy is removing buttons, not adding tabs. Scoring happens at stoppages, which is the same argument that puts set piece behind a tab.

**2. The match clock is the referee's clock, and it is driven by events.** `period_started`, `clock_paused`, `clock_resumed`, `period_ended` and `match_ended` are `control`-group kinds, so the clock survives a reload and two devices tagging the same match agree on it without a merge rule. A device runs a local timer between control events purely to stamp `ClockMs`; that timer's wall-clock anchor is local state and is never synced.

Stopping it works the way a referee stops time: one tap, one fixed position, reachable without leaving whichever tab the coach is on — for an injury, a TMO review, a scrum reset, a slow substitution, and at the end of a half. Nothing accumulates while it is stopped, and that is the point rather than a side effect. Territory, possession and minutes played all use clock-running time as their denominator, so a six-minute injury break cannot dilute a territory percentage or hand a replacement minutes they spent watching.

Because the clock genuinely stops, the screen has to *look* stopped. A paused clock that still reads like a running one is how a coach tags five minutes of play that lands on no timeline at all.

**3. `ClockMs` is continuous and monotonic from kickoff.** Period 2 begins at the value period 1 stopped at — 2,400,000 ms when the half was managed normally. This is what makes 20-minute time slices a single grouping with no new data, exactly as spec section 8 claims, and `Period` stays on the event for display and for extra time.

The monotonicity is load-bearing. The tempting alternative — a first half that overruns to 41:30 and a second half that resets to 40:00 — would send the fold's spans negative and make its sort order meaningless. It is also unnecessary, because of decision 2: the referee stops the clock at 40:00 and plays on to the next stoppage, so the overrun is tagged with the clock stopped. That is both what the stadium clock does and what keeps the timeline monotonic, and it is why the tagging screen stops the clock at 40:00 and 80:00 by itself.

**4. Sin-bin time is running-clock time.** A yellow card removes the player for ten minutes of *clock-running* time, walked across pause/resume intervals. Ten minutes of wall clock would expire during the half-time break and hand the coach a minutes-played figure that is quietly wrong — which spec section 6 names as the reason cards are modelled at all.

Two smaller deviations, noted so they are not mistaken for drift:

- **Event routes nest under the team**, `/api/teams/{teamID}/matches/{matchID}/events`, not the spec's `/api/matches/{id}/events`. The team segment is what `teamFromPath` already checks membership against, and it is the Firestore path.
- **Per-slice figures cover score, counts, territory and possession — not per-player minutes.** Per-player time slicing is a season-view concern and arrives with M4.

---

## File Structure

**M2 adds:**

| File | Responsibility |
|---|---|
| `internal/core/event.go` | `Event`, `EventKind`, `Zone`, `Possession`, validation. Pure. |
| `internal/core/catalogue.go` | The event catalogue as data; kind lookup. Pure. |
| `internal/core/stats.go` | `Fold`, `MatchState`, `PlayerStats`, `PlayerMatchStats`. Pure. |
| `internal/match/service.go` | Append and read use-cases; declares `EventStore`, `EventReader`, `StatsWriter` |
| `internal/match/memstore_test.go` | In-memory stores for service tests |
| `internal/store/firestore/event.go` | Event log persistence and the `since` cursor |
| `internal/store/firestore/playerstats.go` | Derived per-player documents |
| `internal/httpapi/events.go` | Append, list-since and state handlers |
| `internal/httpapi/catalogue.go` | Catalogue handler |
| `testdata/golden/*.json` | Fold fixtures, executed by Go **and** Vitest |
| `web/src/lib/db.ts` | Dexie schema: events, cached match, players, meta |
| `web/src/lib/uuid.ts` | UUIDv7 generation |
| `web/src/lib/fold.ts` | The TypeScript fold |
| `web/src/lib/sync.ts` | Outbox push, cursor pull, status |
| `web/src/lib/pwa.ts` | Service worker registration |
| `web/src/features/tagging/**` | The tagging screen |
| `web/e2e/offline-tagging.spec.ts` | The one end-to-end test |

---

## Task 1: Event and catalogue domain

**Files:**
- Create: `internal/core/event.go`, `internal/core/catalogue.go`
- Test: `internal/core/event_test.go`, `internal/core/catalogue_test.go`

**Interfaces:**
- Consumes: nothing. `internal/core` imports only the standard library.
- Produces:
  - `core.Event{ID, Kind, PlayerID string; ClockMs, Period int; Payload map[string]string; DeviceID, VoidsID string}`
  - `core.EventKind` constants for all 31 tagged kinds plus 5 control kinds
  - `core.EventGroup` constants: `GroupPlay`, `GroupSetPiece`, `GroupDiscipline`, `GroupScore`, `GroupControl`
  - `core.Zone` constants: `ZoneOur22`, `ZoneOurHalf`, `ZoneTheirHalf`, `ZoneTheir22`
  - `core.Possession` constants: `PossessionUs`, `PossessionThem`
  - `core.AllEventKinds()`, `core.AllEventGroups()`, `core.AllZones()`, `core.AllPossessions()`
  - `core.CatalogueEntry{Kind, Label, Group, Player, Points}`, `core.Catalogue() []CatalogueEntry`, `core.LookupKind(EventKind) (CatalogueEntry, bool)`
  - `(Event).Validate(squad map[string]bool) error`

Note `Payload map[string]string`, not `map[string]any`. Every payload this catalogue needs — a zone, a possession, two player IDs — is a string, and a typed map removes an entire class of JSON round-trip mismatch between the two folds.

A stoppage reason on `clock_paused` (injury, TMO, scrum reset) was considered and left out. It would put a picker in front of the one control that has to stay a single tap, and an optional field nobody fills in mid-match is worse than no field at all. If ball-in-play analysis later wants it, it is an additive payload key and a fold that ignores it — no migration.

- [x] **Step 1: Write the failing event validation test**

Create `internal/core/event_test.go`. Cover, as table cases against a squad of two known player IDs:

| Case | Expectation |
|---|---|
| A valid `tackle_made` by a squad player | no error |
| An unknown kind | `ValidationError` on `kind` |
| `tackle_made` with an empty `PlayerID` | `ValidationError` on `playerId` |
| `tackle_made` by a player outside the squad | `ValidationError` on `playerId` |
| `zone_changed` carrying a `PlayerID` | `ValidationError` — team-level kinds take no player |
| `zone_changed` with a valid zone and possession payload | no error |
| `zone_changed` with an unknown zone | `ValidationError` on `payload.zone` |
| `sub` where both payload IDs are in the squad | no error |
| `sub` where `onPlayerId` equals `offPlayerId` | `ValidationError` |
| `sub` missing `offPlayerId` | `ValidationError` |
| `void` with a `VoidsID` | no error |
| `void` with no `VoidsID` | `ValidationError` on `voidsId` |
| `tackle_made` carrying a `VoidsID` | `ValidationError` — only a void cancels |
| Negative `ClockMs` | `ValidationError` on `clockMs` |
| `Period` of 0 or 5 | `ValidationError` on `period` |
| Empty `ID` or empty `DeviceID` | `ValidationError` |

- [x] **Step 2: Write the failing catalogue test**

Create `internal/core/catalogue_test.go`, asserting:

- Every kind returned by `AllEventKinds()` has exactly one `Catalogue()` entry, and every entry's kind is in `AllEventKinds()`. This is the check that catches a kind added to one list and forgotten in the other.
- No duplicate kinds and no empty labels.
- Every entry's `Group` is one of `AllEventGroups()`.
- The PLAY group holds exactly the thirteen kinds spec section 6 lists, by name. Hard-code the expected list: the spec calls thirteen buttons the practical ceiling, so a fourteenth should fail a test rather than reach a coach's thumb.
- Points are `5` for `try` and `opposition_try`, `2` for both conversions-made, `3` for penalty and drop goals, `0` everywhere else.
- Every `score`-group kind that is ours is player-attributed and every opposition kind is not.

- [x] **Step 3: Run both tests to verify they fail**

Run: `go test ./internal/core/`
Expected: FAIL — `undefined: Event`, `undefined: Catalogue`.

- [x] **Step 4: Implement `internal/core/event.go`**

```go
package core

import "strconv"

// Zone is the part of the pitch play is in, from the tagging team's point of
// view. It is a running toggle rather than a per-event location tag: time
// accumulates in the current zone, which produces a territory figure for almost
// no tagging effort.
type Zone string

const (
	ZoneOur22     Zone = "our_22"
	ZoneOurHalf   Zone = "our_half"
	ZoneTheirHalf Zone = "their_half"
	ZoneTheir22   Zone = "their_22"
)

type Possession string

const (
	PossessionUs   Possession = "us"
	PossessionThem Possession = "them"
)

func AllZones() []Zone { ... }          // pitch order, our 22 first
func AllPossessions() []Possession { ... }

func (z Zone) Valid() bool { ... }
func (p Possession) Valid() bool { ... }

// Payload keys. Payload values are always strings: the map round-trips through
// JSON in two languages and a number would arrive as a float64 on one side.
const (
	PayloadZone       = "zone"
	PayloadPossession = "possession"
	PayloadOnPlayer   = "onPlayerId"
	PayloadOffPlayer  = "offPlayerId"
)

// Periods 1 and 2 are the halves; 3 and 4 are extra time.
const (
	firstPeriod = 1
	lastPeriod  = 4
)

// Event is one immutable entry in a match's log. Nothing is ever mutated or
// deleted: a mis-tap is cancelled by appending a void that references it, which
// keeps every derived figure recomputable and leaves an audit trail of what was
// corrected after the match.
type Event struct {
	ID       string            `json:"id" firestore:"-"`
	Kind     EventKind         `json:"kind" firestore:"kind"`
	PlayerID string            `json:"playerId,omitempty" firestore:"playerId"`
	ClockMs  int               `json:"clockMs" firestore:"clockMs"`
	Period   int               `json:"period" firestore:"period"`
	Payload  map[string]string `json:"payload,omitempty" firestore:"payload"`
	DeviceID string            `json:"deviceId" firestore:"deviceId"`
	VoidsID  string            `json:"voidsId,omitempty" firestore:"voidsId"`
}

// Validate checks the event against the catalogue and against the squad selected
// for this match. An event naming a player who is not in the lineup would give
// that player minutes they never played, so it is rejected on arrival rather
// than discovered in a season report.
func (e Event) Validate(squad map[string]bool) error { ... }
```

`Validate` in order: `ID`, `DeviceID`, `ClockMs`, `Period`, kind lookup, player attribution against `entry.Player` and `squad`, `VoidsID` against `KindVoid`, then a switch on the two kinds with structured payloads (`KindSub`, `KindZoneChanged`).

- [x] **Step 5: Implement `internal/core/catalogue.go`**

```go
package core

// EventKind identifies one kind of tagged occurrence. The set is closed and
// lives here: a new statistic is a new kind plus a case in a pure function, with
// nothing in store/ or httpapi/ changing.
type EventKind string

// PLAY — always visible, high frequency.
const (
	KindTackleMade     EventKind = "tackle_made"
	KindTackleMissed   EventKind = "tackle_missed"
	KindCarry          EventKind = "carry"
	KindLinebreak      EventKind = "linebreak"
	KindDefenderBeaten EventKind = "defender_beaten"
	KindOffload        EventKind = "offload"
	KindClearout       EventKind = "clearout"
	KindJackalWon      EventKind = "jackal_won"
	KindTurnoverWon    EventKind = "turnover_won"
	KindHandlingError  EventKind = "handling_error"
	KindKick           EventKind = "kick"
	KindTry            EventKind = "try"
	KindSub            EventKind = "sub"
)

// SET PIECE — occurs at stoppages, so one extra tap is affordable.
const (
	KindScrumWon              EventKind = "scrum_won"
	KindScrumLost             EventKind = "scrum_lost"
	KindScrumPenaltyWon       EventKind = "scrum_penalty_won"
	KindScrumPenaltyConceded  EventKind = "scrum_penalty_conceded"
	KindLineoutWon            EventKind = "lineout_won"
	KindLineoutLost           EventKind = "lineout_lost"
	KindThrowNotStraight      EventKind = "throw_not_straight"
	KindMaulFromLineout       EventKind = "maul_from_lineout"
	KindRestartReceived       EventKind = "restart_received"
	KindRestartLost           EventKind = "restart_lost"
)

// DISCIPLINE — the two cards have side effects in the fold.
const (
	KindOffside     EventKind = "offside"
	KindRuckOffence EventKind = "ruck_offence"
	KindHighTackle  EventKind = "high_tackle"
	KindNotReleasing EventKind = "not_releasing"
	KindScrumOffence EventKind = "scrum_offence"
	KindFoulPlay    EventKind = "foul_play"
	KindYellowCard  EventKind = "yellow_card"
	KindRedCard     EventKind = "red_card"
)

// SCORE — see "Decisions taken here": tries alone cannot produce a scoreboard.
const (
	KindConversionMade      EventKind = "conversion_made"
	KindConversionMissed    EventKind = "conversion_missed"
	KindPenaltyGoal         EventKind = "penalty_goal"
	KindDropGoal            EventKind = "drop_goal"
	KindOppositionTry        EventKind = "opposition_try"
	KindOppositionConversion EventKind = "opposition_conversion"
	KindOppositionPenalty    EventKind = "opposition_penalty"
	KindOppositionDrop       EventKind = "opposition_drop"
)

// CONTROL — the clock and the log's own bookkeeping. No buttons on a tab.
const (
	KindPeriodStarted EventKind = "period_started"
	KindClockPaused   EventKind = "clock_paused"
	KindClockResumed  EventKind = "clock_resumed"
	KindPeriodEnded   EventKind = "period_ended"
	KindMatchEnded    EventKind = "match_ended"
	KindZoneChanged   EventKind = "zone_changed"
	KindVoid          EventKind = "void"
)

type EventGroup string

const (
	GroupPlay       EventGroup = "play"
	GroupSetPiece   EventGroup = "set_piece"
	GroupDiscipline EventGroup = "discipline"
	GroupScore      EventGroup = "score"
	GroupControl    EventGroup = "control"
)

// CatalogueEntry is the configuration for one kind: what it is called, where it
// appears, whether it belongs to a player, and what it is worth. Served to the
// client so that adding a kind is a redeploy of one binary rather than a
// frontend change — and so a second club can eventually be given a different
// catalogue without a fork.
type CatalogueEntry struct {
	Kind   EventKind  `json:"kind"`
	Label  string     `json:"label"`
	Group  EventGroup `json:"group"`
	Player bool       `json:"player"`
	Points int        `json:"points,omitempty"`
}

func Catalogue() []CatalogueEntry { ... }   // in tab order within each group
func AllEventKinds() []EventKind { ... }    // derived from Catalogue()
func LookupKind(k EventKind) (CatalogueEntry, bool) { ... }
```

`AllEventKinds()` derives from `Catalogue()` so the two can never disagree; the test in step 2 still guards the inverse direction. `LookupKind` reads a package-level map built once from `Catalogue()` in an initialiser function — not `init()`; a `sync.OnceValue` or a plain package `var` assigned from a function literal, consistent with `positionGroups`.

- [x] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/core/ -race`
Expected: PASS.

- [x] **Step 7: Register the four new enums with gen-types**

`internal/core` now exports `EventKind`, `EventGroup`, `Zone` and `Possession`, and tygo renders every named string type as `export type X = string`. `assertNoWideAliases` therefore fails `make types` — the guard working as designed — from this task onward, not from task 6. Since CI's `types` job deploys from `main`, the registration has to land in the same commit as the types it describes. Add to the `corePath` union map in `cmd/gen-types/main.go`:

```go
"EventKind":  strs(core.AllEventKinds()),
"EventGroup": strs(core.AllEventGroups()),
"Zone":       strs(core.AllZones()),
"Possession": strs(core.AllPossessions()),
```

Then `make types` and confirm the diff is additive only: the four unions plus `Event` and `CatalogueEntry` in `web/src/types/core.ts`, and no change to `auth.ts`.

**Install `web/node_modules` first.** `make types` ends with `npx biome format --write src/types`, and with no local install `npx` resolves a different, long-deprecated registry package called `biome` that silently reformats the generated files with spaces instead of the tabs `biome.json` asks for. CI installs first and would then fail its own `git diff --exit-code web/src/types`. Run `cd web && npm ci` before `make types` on a fresh clone.

- [x] **Step 8: Verify the whole tree is still green**

Run: `go test ./... -race` and, in `web/`, `npx biome ci . && npm run typecheck && npm test -- --run`.
Expected: PASS throughout — the generated additions must not break an existing page.

- [x] **Step 9: Stage the work**

```bash
git add internal/core/event.go internal/core/catalogue.go internal/core/event_test.go internal/core/catalogue_test.go cmd/gen-types/main.go web/src/types/core.ts
```
Commit message: `feat(core): event log types and the event catalogue`

---

## Task 2: The fold

The single most important function in the product. Every number a coach ever sees comes out of it, and it is the thing the TypeScript side must match exactly.

**Files:**
- Create: `internal/core/stats.go`
- Test: `internal/core/stats_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `core.Fold(m Match, events []Event) MatchState`
  - `core.MatchState`, `core.Score`, `core.PlayerStats`, `core.Slice`
  - `core.PlayerMatchStats` and `core.MatchState.PlayerMatchStats(m Match) []PlayerMatchStats`
  - `core.SliceDurationMs` (1,200,000 — 20 minutes)

- [ ] **Step 1: Write the failing fold test**

Create `internal/core/stats_test.go`. Build a helper that returns a `Match` with a full lineup of 15 starters and 8 replacements with predictable IDs (`p1`…`p23`), then cover one behaviour per test function:

1. **An empty log folds to an empty state.** Zero score, clock 0, not running, status `scheduled`, one `PlayerStats` entry per selected player with zero minutes, starters on the pitch and replacements off.
2. **Counts accumulate per player and only for that player.**
3. **Score sums points from both sides.** A try plus a conversion plus a penalty is 10–0; add an opposition try and conversion for 10–7.
4. **The clock derives from control events.** After `period_started`@0 and events up to 600000, `ClockMs` is 600000 and `Running` is true. After `clock_paused`@600000 it is 600000 and not running.
5. **Minutes count only running clock.** Start, run 10 minutes, pause, resume, run 10 more: every starter has 20 minutes. Then the lagging-device case — a second device stamps an event with a `ClockMs` five minutes into the stoppage, because it had not yet pulled the pause — and the answer is still 20 minutes, not 25. A span while the clock is stopped contributes nothing no matter who stamped it.
6. **A substitution splits minutes.** `sub` at 40:00 with `offPlayerId: p7`, `onPlayerId: p18` — after a full 80, p7 has 40 minutes and p18 has 40, and p18 is on the pitch while p7 is not.
7. **A yellow card removes ten minutes of running clock.** Card at 10:00, pause at 15:00, resume at 20:00, full time at 80:00 → the player has 80 − 10 = 70 minutes of the 75 running minutes, and the five paused minutes did not count against the sin-bin.
8. **A red card removes the player permanently.** Card at 20:00 → 20 minutes, off the pitch at full time, and no later substitution brings them back unless a `sub` says so.
9. **A void removes its target's effect entirely.** Three tackles then a void of the second → count of 2, and the voided ID appears in `VoidedIDs`. Voiding a `sub` restores the original pair's minutes; voiding a try lowers the score.
10. **A void of a void is not a resurrection.** Voiding a void leaves the original event cancelled. State this rule in a comment: the tagging UI only ever voids live events, and a cancel-the-cancel rule would make the log's meaning depend on read order.
11. **Territory and possession accumulate between toggles and only while the clock runs.** A six-minute stoppage in our own 22 leaves the split exactly where it was — the property that makes the percentage worth showing a coach at all.
12. **Order-independence.** Shuffle the event slice with a fixed-seed permutation and assert the folded state is identical. This is the property that makes two coaches tagging one match safe.
13. **Time slices bucket by 20 minutes and split spans across boundaries.** A zone that holds from 19:00 to 22:00 puts 60,000 ms in slice 0 and 120,000 ms in slice 1.
14. **Status follows the log.** No events → `scheduled`; any tagged event → `in_progress`; a `match_ended` event → `completed`.
15. **An event naming an unselected player is ignored, not fatal.** `Fold` has no error return; a player who was removed from the lineup after being tagged must not panic or invent a stats row.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/`
Expected: FAIL — `undefined: Fold`.

- [ ] **Step 3: Implement the state types**

```go
package core

import "time"

// SliceDurationMs is the width of a time-slice bucket. ClockMs is continuous
// from kickoff, so bucketing every metric into 20-minute blocks is one grouping
// here and needs no extra data, no extra writes and no schema change.
const SliceDurationMs = 20 * 60 * 1000

type Score struct {
	Us   int `json:"us"`
	Them int `json:"them"`
}

// PlayerStats is one player's contribution to one match. Counts holds only
// non-zero entries so that the Go and TypeScript folds serialise identically.
type PlayerStats struct {
	PlayerID  string            `json:"playerId"`
	Jersey    int               `json:"jersey"`
	Position  Position          `json:"position"`
	Group     PositionGroup     `json:"group"`
	OnPitch   bool              `json:"onPitch"`
	MinutesMs int               `json:"minutesMs"`
	Counts    map[EventKind]int `json:"counts"`
}

// Slice is one 20-minute block. Per-player slicing is deliberately absent: it is
// a season-view concern and would multiply this structure by 23 for no reader.
type Slice struct {
	FromMs       int                `json:"fromMs"`
	ToMs         int                `json:"toMs"`
	Score        Score              `json:"score"`
	TerritoryMs  map[Zone]int       `json:"territoryMs"`
	PossessionMs map[Possession]int `json:"possessionMs"`
	Counts       map[EventKind]int  `json:"counts"`
}

// MatchState is the whole state of a match, derived and never stored as truth.
type MatchState struct {
	Status       MatchStatus            `json:"status"`
	Period       int                    `json:"period"`
	ClockMs      int                    `json:"clockMs"`
	Running      bool                   `json:"running"`
	Score        Score                  `json:"score"`
	Zone         Zone                   `json:"zone"`
	Possession   Possession             `json:"possession"`
	TerritoryMs  map[Zone]int           `json:"territoryMs"`
	PossessionMs map[Possession]int     `json:"possessionMs"`
	Players      map[string]PlayerStats `json:"players"`
	Slices       []Slice                `json:"slices"`
	VoidedIDs    []string               `json:"voidedIds"`
}

// PlayerMatchStats is the derived per-player document. It denormalises team,
// season, player and date so a season trend or a squad comparison is a
// collection-group query rather than a rollup table that can go stale.
type PlayerMatchStats struct {
	PlayerID  string         `json:"playerId" firestore:"playerId"`
	MatchID   string         `json:"matchId" firestore:"matchId"`
	TeamID    string         `json:"teamId" firestore:"teamId"`
	SeasonID  string         `json:"seasonId" firestore:"seasonId"`
	MatchDate time.Time      `json:"matchDate" firestore:"matchDate"`
	Jersey    int            `json:"jersey" firestore:"jersey"`
	Position  Position       `json:"position" firestore:"position"`
	Group     PositionGroup  `json:"group" firestore:"group"`
	MinutesMs int            `json:"minutesMs" firestore:"minutesMs"`
	Counts    map[string]int `json:"counts" firestore:"counts"`
}
```

`PlayerMatchStats.Counts` is `map[string]int` rather than `map[EventKind]int` because the Firestore client maps arbitrary keys only for `string`-keyed maps.

- [ ] **Step 4: Implement `Fold`**

Structure it as five passes over the log, each small enough to read:

```go
// Fold derives the whole state of a match from its event log. It is pure and
// total: no clock, no I/O, no error. Feed it the same events in any order and it
// returns the same state, which is what makes two coaches tagging one match and
// a correction after the fact both safe.
//
// Time accumulates up to the clock of the last event in the log. For a finished
// match that is exact; during live tagging per-player minutes therefore lag by
// one tap, which no coach is reading mid-match.
func Fold(m Match, events []Event) MatchState {
	voided := voidedIDs(events)           // pass 1: collect cancellations
	ordered := sortedLive(events, voided) // pass 2: drop voided, sort by (ClockMs, ID)
	state := newState(m)                  // pass 3: seed players from the lineup
	...                                   // pass 4: walk the log, span by span
	return state
}
```

The walk maintains, in local variables rather than on `MatchState`:

- `running bool`, `lastClock int` — the span currently open.
- `zone Zone`, `possession Possession` — current toggle state, seeded to `ZoneOurHalf` and `PossessionUs`.
- `onPitch map[string]bool` — who is on the pitch right now.
- `binBudget map[string]int` — for a sin-binned player, how much running clock must still elapse before they return.

Before applying an event, advance time by one span: if `running`, take `e.ClockMs - lastClock` and add it to the current zone, the current possession, the matching slices, **every player in `onPitch`**, and every sin-bin budget; then set `lastClock = e.ClockMs`. If `running` is false the span is skipped entirely — that is the whole of the pause behaviour, and it is why a stoppage cannot leak into a single denominator.

Accumulating minutes span by span, rather than opening and closing an interval per player, is what keeps stoppages, substitutions and sin-bins from needing three separate rules. A pause stops accumulation for everyone at once; a sub changes who is in `onPitch`; a card removes one player and sets a budget. Nothing has to be closed and re-opened, and no code path can forget that the clock was stopped.

A span that crosses a 20-minute boundary is split by a helper:

```go
// addSpan attributes a duration to every slice it overlaps, so a span that runs
// from 19:00 to 22:00 lands 60s in one bucket and 120s in the next.
func addSpan(fromMs, toMs int, add func(slice, ms int))
```

Then switch on the kind. Only six cases do anything beyond counting:

| Kind | Effect |
|---|---|
| `period_started`, `clock_resumed` | `running = true`; `Period` from the event |
| `clock_paused`, `period_ended` | `running = false` |
| `match_ended` | `running = false`; `Status = MatchCompleted` |
| `zone_changed` | set `zone` and `possession` from the payload, ignoring an unknown value |
| `sub` | swap the pair in `onPitch` |
| `yellow_card` / `red_card` | remove the player from `onPitch`; yellow sets a 10-minute running budget, red leaves them off for good |

Everything else increments `Players[e.PlayerID].Counts[e.Kind]`, the slice's counts, and — when `entry.Points > 0` — `Score.Us` or `Score.Them` depending on whether the kind is an opposition kind. Derive that from the catalogue group plus the `Player` flag rather than a second list of names: an opposition kind is a `GroupScore` entry with `Player == false`.

Guard rails to write in, because each is a real failure mode:

- A negative or backwards `ClockMs` span contributes zero, never a negative duration.
- A `sub` whose outgoing player is already off still brings the incoming player on.
- A player tagged but absent from the lineup is skipped, with no map entry created.
- `Counts`, `TerritoryMs`, `PossessionMs` and `Slices` are always non-nil, and zero-valued count entries are never written.

- [ ] **Step 5: Implement `PlayerMatchStats`**

```go
// PlayerMatchStats projects the folded state into the derived documents, keyed
// per player, that season queries read.
func (s MatchState) PlayerMatchStats(m Match) []PlayerMatchStats
```

Sort by jersey so the output is deterministic and a batch write is stable.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/core/ -race -count=1`
Expected: PASS.

- [ ] **Step 7: Stage the work**

```bash
git add internal/core/stats.go internal/core/stats_test.go
```
Commit message: `feat(core): fold the event log into match state`

---

## Task 3: Golden fixtures and the Go runner

The contract that stops the two folds from drifting. Written before the TypeScript fold exists, so that implementation has a target rather than a reference implementation to copy line by line.

**Files:**
- Create: `testdata/golden/README.md`, `testdata/golden/*.json`
- Test: `internal/core/golden_test.go`

**Interfaces:**
- Produces: a JSON fixture format both test suites read.

- [x] **Step 1: Define the fixture format**

Create `testdata/golden/README.md` documenting the shape and the rule that both suites must load every file in the directory, so adding a fixture extends both languages' coverage at once:

```json
{
  "name": "a yellow card costs ten minutes of running clock",
  "match": {
    "seasonId": "2026-27",
    "opponent": "Lansdowne",
    "kickoffAt": "2026-09-19T14:00:00Z",
    "venue": "home",
    "status": "scheduled",
    "lineup": { "starters": [{ "jersey": 1, "playerId": "p1" }], "bench": [] }
  },
  "events": [{ "id": "…", "kind": "period_started", "clockMs": 0, "period": 1, "deviceId": "d1" }],
  "expected": { "status": "in_progress", "clockMs": 4800000, "…": "…" }
}
```

- [x] **Step 2: Write the failing golden runner**

Create `internal/core/golden_test.go`:

- Walk `../../testdata/golden/*.json` with `os.ReadDir`, one subtest per file named after the file.
- Decode into a local struct holding `Name`, `Match`, `Events` and `expected json.RawMessage`.
- `Fold`, marshal the result, and compare against `expected` **after canonicalising both sides** through `map[string]any` round-trips so key order and whitespace do not matter. Use `reflect.DeepEqual` on the decoded maps and, on failure, print both re-marshalled with `json.MarshalIndent` — a diff of two 40-line objects is the only way this failure is debuggable.
- Fail the whole test if the directory is empty or unreadable. A golden suite that silently runs zero cases is worse than no suite.

- [x] **Step 3: Run it to verify it fails**

Run: `go test ./internal/core/ -run Golden`
Expected: FAIL — no fixtures yet.

- [x] **Step 4: Write the fixtures**

Six files, each the smallest log that pins one behaviour. Generate the `expected` blocks by writing the fixture with an empty `expected`, running the runner with a temporary flag or a `t.Log` of the marshalled state, and pasting the result back — then **read it and check it by hand against the rugby**, because a golden file blessed without being read pins a bug in two languages instead of one.

| File | Pins |
|---|---|
| `01-empty.json` | An unplayed fixture: zeroed state, 23 player rows, starters on the pitch |
| `02-counts-and-score.json` | Counts per player; 10–7 from a try, conversion, penalty and an opposition try plus conversion |
| `03-clock-and-minutes.json` | A stoppage mid-half; a first half that plays on past 40:00 with the clock stopped; period 2 resuming at 2,400,000; 80 minutes of running clock for a starter who played throughout |
| `04-sub-and-cards.json` | A substitution splitting minutes, a yellow spanning a pause, and a red |
| `05-void.json` | A void removing a count, a void removing a try from the score, and `voidedIds` |
| `06-territory-and-slices.json` | Zone and possession spans crossing a 20-minute boundary in both directions |

- [x] **Step 5: Run the runner to verify it passes**

Run: `go test ./internal/core/ -run Golden -v`
Expected: PASS, with six named subtests listed. Confirm all six appear — a typo'd glob that matches nothing still passes a poorly written runner, which step 2's empty-directory guard exists to prevent.

- [x] **Step 6: Stage the work**

```bash
git add testdata/golden internal/core/golden_test.go
```
Commit message: `test(core): golden fold fixtures shared with the web client`

---

## Task 4: Match service

**Files:**
- Create: `internal/match/service.go`, `internal/match/memstore_test.go`
- Test: `internal/match/service_test.go`

**Interfaces:**
- Consumes (declared here, implemented in `store/firestore`):
  - `MatchReader{ Match(ctx, clubID, teamID, matchID string) (core.Match, error) }`
  - `MatchWriter{ PutMatch(ctx, clubID, teamID string, m core.Match) error }`
  - `EventStore{ AppendEvents(ctx, clubID, teamID, matchID string, events []core.Event) error }`
  - `EventReader{ Events(ctx, clubID, teamID, matchID, since string) ([]core.Event, string, error) }`
  - `StatsWriter{ PutPlayerStats(ctx, clubID, teamID, matchID string, stats []core.PlayerMatchStats) error }`
- Produces:
  - `match.NewService(MatchReader, MatchWriter, EventStore, EventReader, StatsWriter) *Service`
  - `(*Service).Append(ctx, clubID, teamID, matchID string, events []core.Event) (core.MatchState, error)`
  - `(*Service).Events(ctx, clubID, teamID, matchID, since string) (Page, error)` where `Page{Events []core.Event; Cursor string}`
  - `(*Service).State(ctx, clubID, teamID, matchID string) (core.MatchState, error)`
  - `match.MaxAppendBatch = 500`

Five constructor arguments is at the edge of comfortable, but each interface is one or two methods and the alternative is a `Repository` with nine. `main.go` passes the same `*firestore.Store` five times, which is exactly what structural satisfaction is for.

- [x] **Step 1: Write the in-memory stores**

Create `internal/match/memstore_test.go`, following `internal/squad/memstore_test.go`: keyed by club so the tests can prove tenant isolation. The event store keeps an insertion-ordered slice per match and a monotonic counter for the cursor, upserting by event ID so the idempotency test has something real to exercise. Expose a `putErr error` field so the failure paths can be tested without a mock framework.

- [x] **Step 2: Write the failing service test**

Create `internal/match/service_test.go`:

1. **Append validates against the match's lineup.** An event naming an unselected player returns a `core.ValidationError` and writes nothing.
2. **Append rejects an unknown kind** with a `ValidationError`.
3. **Append is idempotent.** Appending the same three events twice leaves three events and the same state.
4. **Append returns the folded state** including the new counts.
5. **Append writes derived player stats**, one document per selected player, with `teamId`, `seasonId` and `matchDate` denormalised from the match.
6. **Append promotes the match status.** A `scheduled` match becomes `in_progress` on the first tagged event, and a `match_ended` event makes it `completed`. A status that has not changed writes no match document — one write saved per tap on a free tier that counts them.
7. **Append rejects a batch over `MaxAppendBatch`** with a `ValidationError`, and rejects an empty batch.
8. **Append on an unknown match** returns `core.ErrNotFound` and writes nothing.
9. **A club cannot append to another club's match**: the same match ID under a different `clubID` is `ErrNotFound`.
10. **Events since a cursor returns only later events** and a cursor that advances; an empty log returns an empty slice and an empty cursor, never nil.
11. **A store failure on the derived write fails the call** — after the events are already durable. Assert the error wraps and mentions the stats write, so the log says which half failed.

- [x] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/match/`
Expected: FAIL — `undefined: NewService`.

- [x] **Step 4: Implement the service**

```go
// Package match holds the live-tagging use-cases: appending to a match's event
// log and reading the state that log implies.
package match

// Append validates a batch against the match's selected squad, appends it, and
// recomputes the derived per-player documents from the whole log.
//
// The recompute is deliberately total rather than incremental. A match is a few
// hundred events, so re-folding costs microseconds, and a bug in the stats
// engine is then fixed by deploying and re-appending rather than by migrating
// totals that have already drifted.
func (s *Service) Append(ctx context.Context, clubID, teamID, matchID string, events []core.Event) (core.MatchState, error) {
	// 1. load the match (for the lineup, the season and the date)
	// 2. build the squad set from Lineup.Starters + Lineup.Bench
	// 3. validate every event; reject the whole batch on the first failure
	// 4. AppendEvents
	// 5. read the whole log back, Fold
	// 6. PutPlayerStats from state.PlayerMatchStats(match)
	// 7. PutMatch only if the folded status differs from the stored one
}
```

Validate the entire batch before writing any of it: a half-applied batch would be indistinguishable from a partial sync and would have the client retrying events the server already rejected.

- [x] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/match/ -race`
Expected: PASS.

- [x] **Step 6: Stage the work**

```bash
git add internal/match
```
Commit message: `feat(match): append events and recompute derived player stats`

---

## Task 5: Firestore event log and derived stats

**Files:**
- Create: `internal/store/firestore/event.go`, `internal/store/firestore/playerstats.go`
- Test: `internal/store/firestore/event_test.go`, `internal/store/firestore/playerstats_test.go`

**Interfaces:**
- Consumes: `match.EventStore`, `match.EventReader`, `match.StatsWriter` — satisfied structurally, never named here.
- Produces: `(*Store).AppendEvents`, `(*Store).Events`, `(*Store).PutPlayerStats`

- [x] **Step 1: Write the failing store tests**

Follow `internal/store/firestore/testing_test.go`: skip unless `FIRESTORE_EMULATOR_HOST` is set, and use a unique club ID per test so cases do not collide.

1. **Appended events read back** with their fields intact, including an empty payload and a set `voidsId`.
2. **Appending the same ID twice leaves one document** with the later content.
3. **`Events` with an empty cursor returns everything in append order.**
4. **`Events` with the returned cursor returns nothing**, then returns exactly the events appended afterwards.
5. **Two events written in one batch both survive a cursor round-trip** — the case where the ordering field ties and the document ID has to break it.
6. **A malformed cursor is a `ValidationError`**, not a 500 and not a silent full replay.
7. **Events are scoped to the match**: the same event IDs under a second match do not leak into the first.
8. **`PutPlayerStats` overwrites** rather than merging, so a re-fold after a void lowers a count instead of leaving the old one.

- [x] **Step 2: Run against the emulator to verify failure**

Run `make emulator` in another shell, then `make test-store`.
Expected: FAIL — `undefined: AppendEvents`.

- [x] **Step 3: Implement the event store**

```go
// eventsCol is a subcollection of the match, so an event can never be read
// without its club and team in the path.
func (s *Store) eventsCol(clubID, teamID, matchID string) *fs.CollectionRef {
	return s.matchesCol(clubID, teamID).Doc(matchID).Collection("events")
}

// eventDoc is the stored shape. It carries recordedAt, which core.Event does not:
// server-assigned arrival order is what a client's `since` cursor walks, and it
// is sync bookkeeping rather than domain data.
type eventDoc struct {
	Kind       core.EventKind    `firestore:"kind"`
	PlayerID   string            `firestore:"playerId"`
	ClockMs    int               `firestore:"clockMs"`
	Period     int               `firestore:"period"`
	Payload    map[string]string `firestore:"payload"`
	DeviceID   string            `firestore:"deviceId"`
	VoidsID    string            `firestore:"voidsId"`
	RecordedAt time.Time         `firestore:"recordedAt,serverTimestamp"`
}
```

`AppendEvents` writes with a `WriteBatch`, `Set` keyed by `event.ID`. Set, not Create: a retry after a flaky response must overwrite identical data rather than fail, which is the whole reason the endpoint is idempotent. A retry does move `recordedAt` later, so a client may be handed an event it has already seen; that is harmless because the client upserts by ID, and it can never move an event *behind* a cursor, so nothing is ever skipped. Document this reasoning in the file — it is the one subtle part of the sync design.

`Events` orders by `recordedAt` ascending. Firestore appends `__name__` to every order-by, so the ordering is total without a composite index, and `StartAfter(ts, docID)` resumes exactly.

```go
// Cursor is "<unixMicros>:<eventID>": the ordering pair Firestore resumes from.
// Microseconds rather than nanoseconds because that is Firestore's own timestamp
// resolution — a nanosecond cursor would claim a precision the store does not
// have.
func encodeCursor(recordedAt time.Time, id string) string
func decodeCursor(cursor string) (time.Time, string, error)  // core.ValidationError on malformed input
```

The returned cursor is built from the last document in the page; an empty page returns the cursor it was given, so a client polling an idle match does not rewind.

- [x] **Step 4: Implement the derived stats store**

`playerStats` as a subcollection of the match, document ID = player ID, written in one `WriteBatch`. The collection name matters: spec section 5's season queries are collection-group queries over `playerStats`, and a different name here silently breaks M4.

- [x] **Step 5: Run the store tests to verify they pass**

Run: `make test-store`
Expected: PASS. Also run `go test ./... -race` and confirm the store tests still skip cleanly with no emulator.

- [x] **Step 6: Stage the work**

```bash
git add internal/store/firestore/event.go internal/store/firestore/playerstats.go internal/store/firestore/event_test.go internal/store/firestore/playerstats_test.go
```
Commit message: `feat(store): event log persistence with a resumable cursor`

---

## Task 6: Endpoints, wiring and generated types

**Files:**
- Create: `internal/httpapi/events.go`, `internal/httpapi/catalogue.go`
- Modify: `internal/httpapi/router.go`, `cmd/server/main.go`, `cmd/gen-types/main.go`
- Test: `internal/httpapi/events_test.go`, `internal/httpapi/catalogue_test.go`
- Generated: `web/src/types/core.ts`

**Interfaces:**
- Consumes: `*match.Service` on `Deps`.
- Produces:
  - `GET /api/catalogue` → `[]core.CatalogueEntry` (`ActionRead`)
  - `GET /api/teams/{teamID}/matches/{matchID}/events?since=` → `{events, cursor}` (`ActionRead`)
  - `POST /api/teams/{teamID}/matches/{matchID}/events` → `core.MatchState` (`ActionTagMatch`)
  - `GET /api/teams/{teamID}/matches/{matchID}/state` → `core.MatchState` (`ActionRead`)

`ActionTagMatch` already exists in `internal/auth/role.go` and is held by `coach` and `admin` but not `analyst` — M2 is the first milestone that uses it, which is also the first test of whether that table was right.

- [ ] **Step 1: Write the failing handler tests**

Follow `internal/httpapi/testing_test.go`:

1. **Append requires `ActionTagMatch`**: an `analyst` gets 403, a `coach` 200. This is the permission table's first real exercise; if it fails, fix the test's expectation only after re-reading spec section 9.
2. **Append returns the folded state** as JSON with the new counts.
3. **A malformed body is 400** with `"malformed request body"`.
4. **A validation failure is 400** carrying the field and reason.
5. **An unknown match is 404.**
6. **A caller who is not a member of the squad is 403** — `teamFromPath` already does this; the test pins that the new routes go through it.
7. **`since` is passed through verbatim** and the response carries the next cursor.
8. **`GET /api/catalogue` returns every entry** and requires authentication.
9. **The request body shape is `{"events": [...]}`**, not a bare array: a top-level array has no room for a later field and every other endpoint here takes an object.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/httpapi/`
Expected: FAIL — `undefined: handleAppendEvents`.

- [ ] **Step 3: Implement the handlers**

Thin: decode → service → encode, no logic, mirroring `internal/httpapi/matches.go`. Add `Match *match.Service` to `Deps` and guard the new routes with `if d.Auth != nil && d.Match != nil`, as the existing groups do.

```go
type appendEventsRequest struct {
	Events []core.Event `json:"events"`
}
```

- [ ] **Step 4: Wire it in `main.go`**

```go
matchService := match.NewService(store, store, store, store, store)
```

Pass it as `Match: matchService`.

- [ ] **Step 5: Regenerate types and verify**

The four string-literal unions were registered in task 1, where the types first appeared. Nothing new needs narrowing here: `MatchState`, `PlayerStats`, `Slice` and `Score` are structs, which tygo renders as interfaces.

Run: `make types`
Expected: `web/src/types/core.ts` gains `MatchState`, `PlayerStats`, `Slice` and `Score`. Then `cd web && npm run typecheck` to confirm nothing existing broke.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./... -race && make types && git diff --exit-code web/src/types`
Expected: PASS with no diff.

- [ ] **Step 7: Stage the work**

```bash
git add internal/httpapi cmd/server/main.go cmd/gen-types/main.go web/src/types
```
Commit message: `feat(api): event append, event stream and catalogue endpoints`

---

## Task 7: PWA shell

M0 shipped no manifest and no service worker, so today the app cannot load without a network at all — which makes offline tagging unreachable regardless of how good the outbox is. This closes that gap.

**Files:**
- Create: `web/src/lib/pwa.ts`, `web/public/icon-192.png`, `web/public/icon-512.png`
- Modify: `web/package.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `internal/httpapi/spa.go`
- Test: `internal/httpapi/spa_test.go`

- [ ] **Step 1: Add the plugin**

```bash
cd web && npm install -D vite-plugin-pwa
```

- [ ] **Step 2: Configure it**

In `web/vite.config.ts`:

```ts
VitePWA({
	registerType: "autoUpdate",
	manifest: {
		name: "pitch-ai",
		short_name: "pitch-ai",
		description: "Rugby performance tracking",
		display: "standalone",
		orientation: "landscape",
		background_color: "#f8fafc",
		theme_color: "#0f172a",
		start_url: "/",
		icons: [
			{ src: "/icon-192.png", sizes: "192x192", type: "image/png" },
			{ src: "/icon-512.png", sizes: "512x512", type: "image/png" },
			{ src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
		],
	},
	workbox: {
		globPatterns: ["**/*.{js,css,html,svg,png,webmanifest}"],
		navigateFallback: "/index.html",
		// The API is never cached. Offline reads come from IndexedDB, which the
		// app controls; a stale HTTP cache would hand the tagging screen a lineup
		// it cannot explain and cannot invalidate.
		navigateFallbackDenylist: [/^\/api\//],
		runtimeCaching: [{ urlPattern: /^\/api\//, handler: "NetworkOnly" }],
	},
})
```

- [ ] **Step 3: Produce the icons**

Two PNGs from `web/public/favicon.svg`. On macOS:

```bash
cd web/public && qlmanage -t -s 512 -o . favicon.svg && mv favicon.svg.png icon-512.png
```

Then a 192px copy with `sips -Z 192 icon-512.png --out icon-192.png`. Any 512px and 192px export works — the artwork is not load-bearing, but the files must exist or the install prompt never appears on an iPad. Verify both are real PNGs with `file web/public/icon-*.png`.

- [ ] **Step 4: Add the iOS home-screen tags**

`display: standalone` in the manifest is ignored by iOS Safari, which reads its own tags. In `web/index.html`'s `<head>`:

```html
<link rel="apple-touch-icon" href="/icon-192.png" />
<meta name="apple-mobile-web-app-capable" content="yes" />
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
<meta name="theme-color" content="#0f172a" />
```

The iPad is the primary tagging device, so treat a missing home-screen icon there as a bug, not a polish item.

- [ ] **Step 5: Register the worker**

Create `web/src/lib/pwa.ts` wrapping `registerSW` from `virtual:pwa-register` with `immediate: true`, and call it from `main.tsx` before rendering. Add `"vite-plugin-pwa/client"` to the `types` array in `web/tsconfig.app.json` so the virtual module resolves under `strict`.

Guard registration so it is a no-op when `navigator.serviceWorker` is absent — jsdom has no service worker and every existing component test would otherwise start failing on import.

- [ ] **Step 6: Add cache headers to the SPA handler**

Write a failing case in `internal/httpapi/spa_test.go` first: `sw.js` and `index.html` respond with `Cache-Control: no-cache`, and a path under `/assets/` responds with `public, max-age=31536000, immutable`. Then implement it in `spa.go`.

A service worker cached by a CDN or a browser heuristic is how a PWA gets stuck on an old bundle for a week, and the hashed-asset header is the counterpart that makes `no-cache` on the shell cheap.

- [ ] **Step 7: Verify offline loading for real**

```bash
make build && GOOGLE_CLOUD_PROJECT=<project> ./bin/server
```

In Chrome: load the app, then DevTools → Application → Service Workers shows one activated worker; Network → Offline; reload. Expected: the shell renders rather than the browser's offline page.

`RESULT:`/`MATCHES:` this one explicitly — it is the only step in the task that proves the others were worth doing.

- [ ] **Step 8: Stage the work**

```bash
git add web/package.json web/package-lock.json web/vite.config.ts web/index.html web/tsconfig.app.json web/src/main.tsx web/src/lib/pwa.ts web/public/icon-192.png web/public/icon-512.png internal/httpapi/spa.go internal/httpapi/spa_test.go
```
Commit message: `feat(web): installable PWA shell that loads offline`

---

## Task 8: Local event store

**Files:**
- Create: `web/src/lib/db.ts`, `web/src/lib/uuid.ts`
- Modify: `web/package.json`
- Test: `web/src/lib/db.test.ts`, `web/src/lib/uuid.test.ts`

**Interfaces:**
- Produces:
  - `uuidv7(): string`
  - `db` — the Dexie instance
  - `deviceId(): Promise<string>`
  - `appendEvent(matchId, event): Promise<void>`, `localEvents(matchId)`, `unsyncedEvents(matchId, limit)`, `markSynced(ids)`
  - `putServerEvents(matchId, events)`, `getCursor(matchId)`, `setCursor(matchId, cursor)`
  - `cacheMatch(match)`, `cachedMatch(matchId)`, `cachePlayers(players)`, `cachedPlayers()`, `cacheCatalogue(entries)`, `cachedCatalogue()`

- [ ] **Step 1: Add the dependencies**

```bash
cd web && npm install dexie dexie-react-hooks && npm install -D fake-indexeddb
```

Register `fake-indexeddb/auto` in `web/src/setupTests.ts` — jsdom has no IndexedDB.

- [ ] **Step 2: Write the failing UUIDv7 test**

Create `web/src/lib/uuid.test.ts`:

- The format matches `/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/` — version 7, RFC variant.
- 10,000 generated IDs are all distinct.
- IDs generated across two timestamps sort lexicographically in time order. Fake the clock with `vi.useFakeTimers()` rather than sleeping.
- Two IDs generated within the same millisecond are still distinct and still sort stably.

Time-ordered IDs are what let the fold sort a log from two devices without coordination, so the ordering property is the one that actually matters.

- [ ] **Step 3: Implement `uuidv7`**

48 bits of Unix milliseconds, version nibble 7, variant bits `10`, the rest from `crypto.getRandomValues`. Keep a module-level counter in the low bits of the random block for same-millisecond monotonicity, and note in a comment that it resets per page load — which is fine, because ordering only needs to be consistent, not globally unique across reloads.

- [ ] **Step 4: Write the failing database test**

Create `web/src/lib/db.test.ts`, clearing the database between cases:

1. `appendEvent` stores the event unsynced and `localEvents` returns it.
2. `unsyncedEvents` respects its limit and returns oldest first.
3. `markSynced` removes events from `unsyncedEvents` while leaving them in `localEvents`.
4. `putServerEvents` upserts by ID: an event that arrived locally and then came back from the server is one row, marked synced.
5. `deviceId` is stable across calls and across a fresh database handle.
6. The cursor and the cached catalogue round-trip.

- [ ] **Step 5: Implement `db.ts`**

```ts
// IndexedDB is the application's state, not a cache in front of it. Every tap
// writes here first and the UI renders from here, so the app behaves identically
// with and without signal.
class PitchDB extends Dexie {
	events!: Table<LocalEvent, string>;
	matches!: Table<Match, string>;
	players!: Table<Player, string>;
	meta!: Table<MetaRow, string>;

	constructor() {
		super("pitch-ai");
		this.version(1).stores({
			// synced is 0/1 rather than a boolean: IndexedDB cannot index booleans,
			// and the outbox query is [matchId+synced].
			events: "id, matchId, [matchId+synced]",
			matches: "id",
			players: "id",
			meta: "key",
		});
	}
}

type LocalEvent = Event & { matchId: string; synced: 0 | 1 };
```

There is no separate outbox table: the `synced` flag on the event row *is* the outbox. One table means one write per tap and no chance of the two drifting.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd web && npm test -- --run`
Expected: PASS.

- [ ] **Step 7: Stage the work**

```bash
git add web/package.json web/package-lock.json web/src/setupTests.ts web/src/lib/db.ts web/src/lib/db.test.ts web/src/lib/uuid.ts web/src/lib/uuid.test.ts
```
Commit message: `feat(web): local event store and UUIDv7 generation`

---

## Task 9: The TypeScript fold

**Files:**
- Create: `web/src/lib/fold.ts`
- Test: `web/src/lib/fold.golden.test.ts`, `web/src/lib/fold.test.ts`

**Interfaces:**
- Produces: `fold(match: Match, events: Event[]): MatchState`, using the generated types from `web/src/types/core.ts` — no hand-written parallel type definitions.

- [ ] **Step 1: Write the failing golden parity test**

Create `web/src/lib/fold.golden.test.ts`:

```ts
// The same fixtures the Go tests run. Drift between the two folds fails CI here.
const goldenDir = new URL("../../../testdata/golden/", import.meta.url);
```

Read the directory with `node:fs`, `describe.each` over the files, and for each: `fold(fixture.match, fixture.events)`, round-trip through `JSON.parse(JSON.stringify(state))`, and `expect(...).toEqual(fixture.expected)`.

Assert the file count is greater than zero, for the same reason the Go runner does.

The round-trip is not ceremony: it is what makes `undefined` versus a missing key, and a `Map` versus an object, fail here rather than in a report three milestones later.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npm test -- --run fold`
Expected: FAIL — cannot resolve `./fold`.

- [ ] **Step 3: Implement `fold.ts`**

Mirror `internal/core/stats.go` pass for pass and name for name — `voidedIds`, `sortedLive`, `addSpan`, the same local variables in the same order. The two implementations will be read side by side every time a fixture disagrees, and a clever TypeScript rewrite makes that comparison expensive for no gain.

Three parity traps to handle explicitly, each with a comment:

- **Sorting.** JavaScript's default `sort` is lexicographic; compare `clockMs` numerically and fall back to a string compare on `id`.
- **Empty collections.** Go emits `{}` for an initialised empty map and `[]` for an initialised empty slice. Build objects and arrays, never `undefined`, and omit zero-valued counts exactly as Go does.
- **Integer division.** Slice index is `Math.floor(clockMs / SLICE_DURATION_MS)`.

Keep the semantics — points per kind, which kinds are cards, subs and toggles — in this file rather than reading them from the served catalogue. The catalogue is presentation configuration and a client may hold a stale copy; the fold's behaviour must not depend on it.

- [ ] **Step 4: Write a small set of TypeScript-only unit tests**

Create `web/src/lib/fold.test.ts` for the two things fixtures cannot pin: that `fold` on an empty log returns a usable state for a match with no lineup at all (the tagging screen renders before an XV is picked), and that it does not mutate its `events` argument (the caller is passing a live Dexie query result).

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npm test -- --run`
Expected: PASS, with all six golden fixtures listed by name.

If a fixture disagrees, **fix the fold, not the fixture** — unless re-reading the Go implementation shows Go is the one that is wrong, in which case fix Go, re-bless that fixture, and note it in the commit message.

- [ ] **Step 6: Stage the work**

```bash
git add web/src/lib/fold.ts web/src/lib/fold.test.ts web/src/lib/fold.golden.test.ts
```
Commit message: `feat(web): TypeScript fold pinned to the shared golden fixtures`

---

## Task 10: Outbox sync

**Files:**
- Create: `web/src/lib/sync.ts`
- Test: `web/src/lib/sync.test.ts`

**Interfaces:**
- Produces:
  - `createSync({ apiFetch, teamId, matchId }): Sync`
  - `Sync{ start(), stop(), push(), pull(), subscribe(cb) }`
  - `SyncStatus{ state: "offline" | "pending" | "syncing" | "synced" | "error"; pending: number; error: string | null }`

- [ ] **Step 1: Write the failing sync test**

Create `web/src/lib/sync.test.ts` with a stub `apiFetch` that records calls and can be told to fail. Use `vi.useFakeTimers()`; no test waits on real time.

1. **`push` drains unsynced events in batches** of at most 50 and marks each batch synced.
2. **A failed push leaves the events unsynced** and reports `state: "error"`, so nothing is lost.
3. **A push retries with backoff** and stops retrying once it succeeds.
4. **A 400 does not retry forever.** A validation rejection is permanent: mark the batch as rejected, surface it, and move on — a poison event must not block every later tap from ever syncing. Record the rejection on the row rather than deleting it; the log is append-only and a silently vanished tap is worse than a visible error.
5. **`pull` requests with the stored cursor, upserts the response, and stores the new cursor.**
6. **`pull` ignores events it already has** (the device's own events coming back) without duplicating a row.
7. **Going offline reports `state: "offline"` and pushes nothing**; the `online` event triggers a push.
8. **`subscribe` fires on every status change** and the returned function unsubscribes.
9. **`stop` cancels the interval** and no further calls happen.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npm test -- --run sync`
Expected: FAIL — cannot resolve `./sync`.

- [ ] **Step 3: Implement `sync.ts`**

Push before pull on every cycle, so a coach's own taps leave the device at the first opportunity. Trigger a cycle on: `start()`, a 10-second interval, the `online` event, `visibilitychange` to visible, and an explicit call after each append. Guard re-entrancy with an in-flight flag — an interval firing during a slow push must not double-send.

```ts
// The endpoint is idempotent: events are keyed by their own ID, so a retry after
// a flaky response overwrites identical data. There is no dedup logic here and
// no "did that send?" ambiguity — the worst case of a retry is a wasted write.
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test -- --run`
Expected: PASS.

- [ ] **Step 5: Stage the work**

```bash
git add web/src/lib/sync.ts web/src/lib/sync.test.ts
```
Commit message: `feat(web): outbox sync with cursor pull and backoff`

---

## Task 11: The tagging screen

**Files:**
- Create: `web/src/features/tagging/TaggingPage.tsx`, `TopStrip.tsx`, `SquadGrid.tsx`, `ActionPane.tsx`, `RecentEvents.tsx`, `useTagging.ts`, `api.ts`
- Modify: `web/src/main.tsx`, `web/src/features/matches/MatchesPage.tsx`
- Test: `web/src/features/tagging/useTagging.test.ts`, `web/src/features/tagging/TaggingPage.test.tsx`

**Interfaces:**
- Produces: the route `/squads/:teamId/matches/:matchId/tag`; `useTagging(...)` returning the armed state, the folded state, and the tap handlers.

Layout, per spec section 7: iPad landscape, dual pane, **no modals**. Left is the squad as numbered tiles with the bench visually distinct; right is the action pane with three tabs; the top strip carries clock, score, sync status, the zone/possession toggles and the score strip; the bottom strip is recent events with a single-tap Undo.

- [ ] **Step 1: Write the failing interaction test**

Create `web/src/features/tagging/useTagging.test.ts`. This is the muscle-memory contract, so test it as a state machine rather than through the DOM:

1. **Action then player logs one event** and leaves the action armed.
2. **An armed action plus three players logs three events** in three taps — spec section 7's worked example, verbatim.
3. **Player then action logs one event** and clears the armed player.
4. **Tapping the same action twice disarms it**, so a coach can back out without logging.
5. **Tapping a second action replaces the armed one.**
6. **A team-level action logs immediately** with no player and no arming.
7. **A player who is not on the pitch cannot be tagged** — assert no event is written.
8. **Undo appends a void** of the most recent non-void, non-control event by this device, and never of another device's event.
9. **Undo with nothing to undo is a no-op.**
10. **The clock controls append control events** — start, pause, resume, end period, end match — and pause then resume leaves the clock running.
11. **While paused, the stamped clock is frozen.** Tag three events during a stoppage and all three carry the same `ClockMs` as the pause, because the device's timer stops with it.
12. **A pause from another device stops this one.** Feed a remote `clock_paused` into the local log: the displayed clock snaps to that event's `ClockMs` and stops ticking, without this device having tapped anything.
13. **The clock stops itself at 40:00 and at 80:00**, appending exactly one `clock_paused` rather than one per tick, and a coach can resume it afterwards for the overrun or for extra time.
14. **Zone and possession toggles append `zone_changed` carrying both values**, because the event is full state rather than a delta.
15. **Every appended event carries this device's ID, the current period and the current clock.**

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npm test -- --run tagging`
Expected: FAIL — cannot resolve `./useTagging`.

- [ ] **Step 3: Implement `api.ts` and cache priming**

`useCatalogue()` and `usePrimedMatch(teamId, matchId)`: fetch from the API when online, write straight into Dexie, and read through `useLiveQuery` so the components only ever render local state. On a cold offline start the fetch fails and the cached copy renders — which is the whole point, so the fetch failure must not surface as an error when a cached copy exists.

- [ ] **Step 4: Implement `useTagging.ts`**

```ts
// Two taps in either order, because on a sideline the coach's eyes are on the
// match. An action stays armed after it fires so repeated taps on players log
// repeated events; a player does not, because the next action is usually
// different.
type Armed = { kind: EventKind | null; playerId: string | null };
```

The live clock: hold `{ clockMs, wallMs, running }` in local state, anchored to the **last control event in the merged log** ordered by `(clockMs, id)` — not to the last control event this device happened to tap. A pause that arrives from the other coach's iPad on the next pull must snap this device's anchor to that event's `clockMs` and stop it, or the two devices go on stamping different times for the same moment. Tick with `setInterval` at 1 Hz for display only; while stopped, the stamped `ClockMs` is simply the anchor.

The anchor itself is never synced — it is a wall-clock detail, and `ClockMs` stamped on each event is the only thing that crosses the wire.

Stop the clock automatically when the running clock reaches 40:00 and 80:00, appending one `clock_paused`. This is what a referee's clock does, it removes a tap at the busiest moment of the half, and per decision 3 it is what keeps `ClockMs` monotonic when the half plays on past 40:00. Guard it so it fires once per boundary rather than once per tick. If it proves wrong in a real match, deleting the effect is a self-contained change — which is the test of whether a behaviour like this belongs in the UI rather than the fold.

- [ ] **Step 5: Implement the components**

Fixed button positions are the requirement; nothing may reflow as state changes. Tailwind utility classes only, matching the existing pages — no scoped styles, no new colour values outside the `slate`/`red` palette already in use.

- `TopStrip` — the clock, the period, the score with the score-group buttons, the four zone buttons and two possession buttons as segmented toggles, and the sync indicator (pending count when non-zero).

  The clock's stop/start is a single button in a fixed position, large enough to hit without looking and never behind a tab: a referee stops time at no notice, and a control that takes two taps to reach will not be used. A stopped clock must be unmistakable at a glance — the digits change colour and stop blinking their separator, and the strip says so in words. Every downstream percentage is denominated in running clock, so a clock the coach believes is running when it is not corrupts the whole match quietly.
- `SquadGrid` — 23 tiles; jersey number large, surname small, live count of the armed action on each tile; the bench in a separate block; a sin-binned or red-carded player visibly out.
- `ActionPane` — three tabs from the catalogue's `play`, `set_piece` and `discipline` groups, in served order. Render the armed action as pressed. Never re-order buttons based on frequency of use: predictable position is the feature.
- `RecentEvents` — the last eight live events, newest first, with one Undo button.

- [ ] **Step 6: Add the route and the entry point**

Route in `main.tsx`; a "Tag" link on each fixture in `MatchesPage.tsx`, shown only when the lineup has 15 starters — there is nothing to tag without a side, and a half-filled lineup would give every count a home but no minutes.

- [ ] **Step 7: Write the page smoke test**

`TaggingPage.test.tsx`: renders with a seeded Dexie database and no network, shows 23 tiles, and tagging a tackle updates the tile's count. One test, proving the wiring; the behaviour is covered in step 1.

- [ ] **Step 8: Run everything**

Run: `cd web && npx biome ci . && npm run typecheck && npm test -- --run`
Expected: PASS.

- [ ] **Step 9: Stage the work**

```bash
git add web/src/features/tagging web/src/main.tsx web/src/features/matches/MatchesPage.tsx
```
Commit message: `feat(web): live tagging screen with offline event capture`

---

## Task 12: Offline end-to-end test

The one Playwright test spec section 12 calls for: tag with the network disabled, restore it, assert everything synced.

**Files:**
- Create: `web/playwright.config.ts`, `web/e2e/offline-tagging.spec.ts`, `web/e2e/stubs.ts`
- Modify: `web/package.json`, `web/src/lib/auth.tsx`, `.github/workflows/ci.yml`, `web/.gitignore`

- [ ] **Step 1: Install Playwright**

```bash
cd web && npm install -D @playwright/test && npx playwright install --with-deps chromium
```

Add `test:e2e` to `package.json` scripts and `/playwright-report`, `/test-results` to `web/.gitignore`.

- [ ] **Step 2: Add the test-only auth stub**

The test exercises the outbox, not Firebase. Sign-in is stubbed at one point, guarded so a production bundle cannot contain it:

```tsx
// Only ever true for the e2e build: VITE_E2E is set by the test:e2e script and
// by nothing else. Vite inlines it at build time, so the production bundle has
// the real branch and no dead stub code.
if (import.meta.env.VITE_E2E === "1") { ... }
```

The stub supplies a fixed membership and a token-less `apiFetch`. Assert its absence in the shipped bundle as part of step 5 rather than trusting the flag.

- [ ] **Step 3: Write the test**

`web/e2e/offline-tagging.spec.ts`, against `vite preview` with `VITE_E2E=1`, with `page.route("**/api/**")` serving fixtures from `stubs.ts` and recording every append:

1. Open a match's tagging screen; the catalogue, match and squad load and cache.
2. `context.setOffline(true)`.
3. Start the clock and tag a known sequence: ten tackles across three players, a try, a conversion, a substitution and one Undo.
4. Assert the screen's own counts are right with the network down — this is the offline-first claim.
5. Reload the page while still offline and assert the counts survive. IndexedDB, not memory.
6. `context.setOffline(false)`.
7. Wait for the sync indicator to report everything sent, then assert the recorded appends contain exactly the expected event IDs and kinds, including the void, with no duplicates.

- [ ] **Step 4: Add the CI job**

A fourth job in `.github/workflows/ci.yml` mirroring the `web` job, running `npx playwright install --with-deps chromium` then `npm run test:e2e`, and add it to the `deploy` job's `needs`.

- [ ] **Step 5: Verify**

Run: `cd web && npm run test:e2e`
Expected: PASS.

Then `npm run build && grep -rc "VITE_E2E" dist/ || echo "clean"` — expected: no match in the production bundle.

- [ ] **Step 6: Stage the work**

```bash
git add web/playwright.config.ts web/e2e web/package.json web/package-lock.json web/.gitignore web/src/lib/auth.tsx .github/workflows/ci.yml
```
Commit message: `test(web): end-to-end offline tagging and sync`

---

## Task 13: Deploy M2

**Files:**
- Modify: `Makefile` (a `test-e2e` target), `docs/superpowers/plans/2026-09-20-m2-live-tagging.md` (check the boxes)

- [ ] **Step 1: Run the whole suite**

```bash
go test ./... -race
make test-store          # with the emulator running
make types && git diff --exit-code web/src/types
cd web && npx biome ci . && npm run typecheck && npm test -- --run && npm run test:e2e
```

- [ ] **Step 2: Add a `test-e2e` target to the Makefile**

Next to `test-web`, so the full local suite is discoverable from one file.

- [ ] **Step 3: Deploy**

Push to `main` and let CI deploy, or `make deploy` for an out-of-band revision. Confirm the GitHub deployment records the URL and `/api/healthz` returns 200.

- [ ] **Step 4: Verify on the real device**

On the iPad, on the deployed URL:

- Add to Home Screen; the icon appears and the app opens without Safari chrome, in landscape.
- Open a fixture with a full XV and tag with Wi-Fi on: counts move, the clock runs.
- Turn Wi-Fi off. Tag twenty more events, use Undo, toggle zone and possession. Everything responds with no spinner and no blocked tap.
- Stop the clock as a referee would, wait two minutes, tag a few events, and restart it. The clock reads the same before and after, those events all carry the stopped time, and no player gained minutes. Confirm at a glance that the stopped clock looked stopped.
- Force-quit the app, reopen it offline: the events are still there.
- Turn Wi-Fi on. The sync indicator drains to zero.
- On a laptop, `GET /api/teams/{teamID}/matches/{matchID}/state` and check the numbers against the iPad's screen.

The last check is the one that matters: the same log folded in two languages on two devices agreeing is the claim M2 exists to make.

- [ ] **Step 5: Stage the work**

```bash
git add Makefile docs/superpowers/plans/2026-09-20-m2-live-tagging.md
```
Commit message: `chore: run the end-to-end suite from the Makefile`

---

## Definition of done

M2 is complete when all of the following are true:

1. `go test ./... -race` passes, and `make test-store` passes against the emulator.
2. `npx biome ci . && npm run typecheck && npm test -- --run && npm run test:e2e` pass in `web/`.
3. `make types` produces no diff.
4. All six golden fixtures pass in both Go and Vitest, and a deliberate one-line change to either fold fails CI in that language.
5. The deployed app installs to an iPad home screen and its shell loads with the network disabled.
6. A coach can tag a full match offline — clock, score, zones, subs, cards, Undo — and every event reaches Firestore once signal returns, with no duplicates and none lost across a force-quit.
7. Stopping the clock stops every denominator: a stoppage adds no minutes, no territory time and no possession time, and the screen shows unmistakably that the clock is not running.
8. `GET …/state` on the server returns the same counts, minutes, score and territory split the iPad showed.
9. An `analyst` account can read a match but is refused the append endpoint with a 403.
10. Appending an event twice leaves one event and identical derived stats.

## What M3 will build on this

M3 (match report) is the next plan and depends on this one for: `core.Fold` and `MatchState` including `Slices`, the `playerStats` subcollection with its denormalised `teamId`, `seasonId` and `matchDate`, the `GET …/state` endpoint, and the event catalogue — to which M3 adds the rollup definitions (tackle completion, set-piece retention) that turn raw counts into the report's KPI tiles.
