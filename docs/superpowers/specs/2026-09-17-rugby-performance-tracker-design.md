# Rugby Performance Tracker — Design

**Date:** 2026-09-17
**Status:** Approved for planning
**Working name:** pitch-ai

## 1. Purpose

Track the performance of a rugby union (15s) team and its individual players, so
that coaches can make better selection decisions and players can see where they
are improving. The system is used on the sideline during a match and at a desk
afterwards.

The goal is not data storage. The goal is that a coach opens the app on Tuesday
and changes what happens at training.

## 2. Users and scope

**Now:** one club, one team, the head coach plus two or three assistant coaches.

**Designed for later:** other clubs adopting the product. Multi-club tenancy is
in the data model and the authorisation rules from day one. Multi-club
*onboarding* — self-serve signup, billing, club administration — is not built.

Devices are mixed: an iPad on the sideline, Android phones and laptops for other
coaches. No user can be assumed to own an Apple device.

## 3. Architecture

Three parts, one deployable artifact.

| Part | Choice |
|---|---|
| Client | React + TypeScript PWA, installed to the home screen |
| Server | Single Go binary on Cloud Run, serving both the API and the embedded web bundle |
| Datastore | Firestore (Native mode) |
| Auth | Firebase Auth (Google sign-in, email link) |

The Go binary embeds the built web assets with `go:embed`. One artifact, one
URL, one deploy, no CORS configuration.

**The browser never talks to Firestore directly.** All tenancy and permission
logic lives in Go, where it can be read and tested in one place, rather than
being split between application code and Firestore security rules. Firestore is
a server-side implementation detail and can be replaced without touching the
client.

### Why this stack

Everything scales to zero. Cloud Run is free below roughly 2M requests/month;
Firestore's free tier (50k reads / 20k writes per day) is far above what a team
generates — a full match is a few hundred events. Expected running cost is
**€0–2/month**, almost entirely Artifact Registry storage.

Cloud SQL was rejected: ~€10–15/month whether or not anyone opens the app, plus
a VPC connector, private IP and patch windows. The analytics queries needed here
are known in advance and few, so SQL's flexibility does not pay for its fixed
cost and ongoing attention.

## 4. Sync model

Offline-first, built on an **append-only event log**. This is the decision that
makes everything else simple.

1. Each tap writes an immutable event to IndexedDB on the device. The UI renders
   from local state. Nothing blocks on the network; the app behaves identically
   with no signal.
2. Each event carries a client-generated **UUIDv7** as its ID — time-ordered and
   collision-free without coordination between devices.
3. A background outbox drains unsynced events in batches to
   `POST /api/matches/{id}/events` whenever connectivity allows.
4. The endpoint is **idempotent**: events are written keyed by their own ID, so a
   retry after a flaky response overwrites identical data. There is no dedup
   logic and no "did that send?" ambiguity.
5. Clients pull with `GET /api/matches/{id}/events?since={cursor}` to see events
   appended by other devices.

**Two coaches tagging the same match works with no merge algorithm.** Appends
from different devices interleave into one ordered log. Nothing is ever mutated,
so there is no last-write-wins data loss.

**Corrections are events.** A mis-tap is fixed by appending a `void` event
referencing the original event's ID. The log stays immutable, derived statistics
recompute, and the result is a free audit trail of what was corrected after the
match.

## 5. Data model

### Firestore layout

```
clubs/{clubId}
  players/{playerId}            # name, DOB, positions[], teamIds[], status
  teams/{teamId}
    matches/{matchId}           # opponent, date, competition, venue, startingXV, status
      events/{eventId}          # the append-only log
      playerStats/{playerId}    # DERIVED — recomputed server-side, never hand-written
users/{uid}                     # clubId, teamIds[], role
```

Players belong to the **club**, not the team, so a colt who plays up for the 1st
XV is one person with one history rather than two records that never reconcile.
Team membership is a list on the player document.

Every write path in Go takes `clubId` resolved from the caller's auth token,
**never** from the request body. This single rule is what makes multi-club safe
later without revisiting the codebase.

### Event

```go
type Event struct {
    ID       string         // UUIDv7, generated on the device
    Kind     string         // from the catalogue, e.g. "tackle_made", "sub", "zone_changed"
    PlayerID string         // empty for team-level events
    ClockMs  int            // match clock, not wall clock
    Period   int            // 1, 2, or extra time
    Payload  map[string]any // zone+possession for toggles, on/off player IDs for subs
    DeviceID string         // which coach's device recorded it
    VoidsID  string         // set when this event cancels an earlier one
}
```

Zone toggles, substitutions and cards are **all events**. There is no separate
territory table, subs table, or card handling.

### The fold

The entire state of a match — score, territory split, who is on the pitch,
minutes played, every per-player count — is a fold over the event log:

```go
func Fold(m Match, events []Event) MatchState   // pure; lives in core/; zero dependencies
```

This function tests with no mocks and no emulator. It also makes corrections
trustworthy: void an event, re-fold, and every downstream number is correct by
construction. No cached total can silently drift out of step with its source log.

### Derived data is a cache and is treated like one

When events land, the server re-folds the **whole** log and overwrites the
per-player documents. Never incremented, never patched — always recomputed from
scratch. A match is a few hundred events, so this costs microseconds, and a bug
in the stats engine is fixed by deploying and re-folding rather than by migrating
corrupted totals.

**No season rollups.** Firestore collection-group queries over `playerStats`
cover it: a player's season trend is ~25 document reads, a full squad comparison
~600. Both are free at this volume and both stay correct automatically. A rollup
layer would buy nothing and add a second thing that can go stale. `playerStats`
documents denormalise `teamId`, `seasonId`, `playerId` and `matchDate` to support
these queries.

### Client/server fold parity

The iPad needs live running counts while offline, so the fold must also exist in
TypeScript. Duplicated logic drifts, so both implementations are pinned to a
shared set of **golden fixtures**: JSON files of `(events in → expected state
out)` checked into the repo and executed by both the Go tests and Vitest. Drift
fails CI.

The alternative — compiling the Go fold to WASM — was rejected as a ~2 MB payload
and a fiddlier build for a client that only needs counts and minutes. The subtler
logic stays server-side.

## 6. Event catalogue

The catalogue — the list of event kinds, which tab each appears on, and how each
rolls up into a statistic — is **configuration data defined in Go and served to
the client at login**, not hard-coded in the frontend. Adding an event kind is a
data change and a redeploy of one binary, not a frontend rewrite. It is also how
a second club eventually gets a slightly different catalogue without forking.

**PLAY tab** (always visible; high frequency):
tackle made, tackle missed, carry, linebreak, defender beaten, offload, clearout,
jackal won, turnover won, handling error, kick, try, substitution.

**SET PIECE tab:**
scrum won, scrum lost, scrum penalty won, scrum penalty conceded, lineout won,
lineout lost, throw not straight, maul from lineout, restart received,
restart lost.

**DISCIPLINE tab:**
offside, ruck offence, high tackle, not releasing, scrum offence, foul play,
yellow card, red card.

**Territory state** is not a per-event location tag. The tagger keeps a running
zone toggle — *our 22 / our half / their half / their 22* — plus a possession
toggle (*us / them*). Time accumulates in each state, producing territory and
possession figures for almost no tagging effort. A change of state appends a
`zone_changed` event.

**Cards have side effects.** A yellow card removes the player from the pitch for
10 minutes in the fold; a red removes them permanently. Without this, minutes
played silently lies and every per-80 rate built on it lies too.

## 7. Live tagging screen

iPad, landscape, held in two hands. Layout: **dual pane, no modals**.

- **Left:** the squad as a grid of numbered tiles — starting XV plus bench,
  bench visually distinct.
- **Right:** the action pane, with the three catalogue tabs.
- **Top:** match clock, score, sync status, and the zone/possession strip.
- **Bottom:** a recent-events strip with a single-tap **Undo**.

Logging an event is two taps — a player and an action — **in either order**.
Because either half can be tapped first, an action can stay *armed*: tap
`Tackle ✓` once, then tap players 1, 4 and 12 to log three tackles in three taps.

A modal sheet was rejected. It covers the pitch-side view, it costs an animation
you cannot tap through, and it breaks the muscle memory that matters when a
coach's eyes are on the match rather than the screen. Fixed button positions are
the point.

Set piece and discipline sit behind tabs because they occur during natural
stoppages, where one extra tap is affordable. Clearout and jackal stay on the
PLAY tab despite being breakdown events, because they are too frequent to hide.

The PLAY tab holds 13 buttons, which is near the practical ceiling for muscle
memory. If live tagging proves too busy in a real match, the fix is removing
buttons from PLAY — a configuration edit — not adding a fourth tab.

## 8. Analytics and reports

Three principles, applied everywhere:

**Rates, not counts.** Every figure is normalised per 80 minutes or expressed as
a completion percentage. A flanker with 14 tackles in 80 minutes and a
replacement with 6 in 22 are statistically equivalent; raw counts say otherwise.
This is why minutes tracking exists.

**Small samples are handled explicitly, not hidden.** Raw numbers always display
alongside the sample they came from. Any *ranking* shrinks a player's rate toward
their position-group mean in proportion to how little they have played, and
flags that it has done so. There is no black box — a coach who cannot reason
about a number will rightly distrust it.

**Comparison happens within position groups:** front row (1–3), second row (4–5),
back row (6–8), half backs (9–10), centres (12–13), back three (11, 14, 15).
Ranking a prop's carry count against a winger's is noise dressed as insight.

### Post-match team report

Leads with **what changed**, not with everything available: three lines
contrasting this match against the season baseline. Below that: territory split
by third, possession share, team KPI tiles with deltas, and a standouts table of
per-80 rates.

The report presents *where to look*. It does not assert causation.

Season-baseline comparison requires no extra machinery — the report is computed
against the same `playerStats` collection the trend pages read.

### Time-slice breakdown

Every metric is also bucketed into 20-minute blocks. `ClockMs` is already on
every event, so this is one additional grouping inside `Fold` — no new data, no
new writes, no schema change.

### Player page

Headline per-80 rates with deltas against the player's own recent form, a
match-by-match trend for the key metric against the position-group average, and a
ranking table within the position group.

### Per-player feedback card

A single-page card per player — their numbers, their trend, their position-group
context — at its own share URL. A filtered render of data the player page already
produces.

### Sharing

Reports and cards are reachable by an unguessable, revocable link, so a squad
member or club official can open one without an account being provisioned.

PDF export is the browser's own print dialog against a print stylesheet. No
headless-Chrome service to run or pay for — server-side PDF rendering is the most
expensive thing commonly bolted onto an application this size.

## 9. Auth and permissions

Firebase Auth with Google sign-in and email-link. Go verifies the ID token on
every request and resolves `clubId` and role from `users/{uid}`.

| Role | Permissions |
|---|---|
| `coach` | Tag matches, create fixtures, set lineups, read all reports |
| `analyst` | Read, and edit match data after the match |
| `admin` | Manage squad, matches, and club membership |

## 10. Privacy

Per-player share links mean individual performance data leaves the coach's
control, and running this for colts means handling minors' data under GDPR.
Mitigations are built in from the start rather than retrofitted:

- Share links **expire** (default 90 days) and are **revocable per player**.
- Shared cards carry **no date of birth and no contact details** — rugby data only.
- Deleting a player removes their documents and invalidates their share links.

## 11. Code structure

One repository, two applications, no monorepo tooling.

```
pitch-ai/
├── cmd/server/main.go        # the only file that knows how everything wires together
├── internal/
│   ├── core/                 # domain types + rules. Zero dependencies.
│   │   ├── match.go          #   Match, Event, EventKind, Lineup
│   │   ├── player.go
│   │   ├── catalogue.go      #   the event catalogue, as data
│   │   └── stats.go          #   Fold and the pure stat functions
│   ├── match/                # use-cases; defines the small interfaces it needs
│   ├── season/               # trends, squad comparison
│   ├── store/firestore/      # implements those interfaces
│   ├── auth/                 # Firebase ID-token verification middleware
│   └── httpapi/              # thin handlers: decode → service → encode. No logic.
├── web/src/
│   ├── features/             # organised by what it does: tagging/ matches/ players/ reports/
│   ├── lib/                  # db.ts (Dexie), sync.ts (outbox), api.ts
│   ├── components/ui/        # shared primitives
│   └── types/api.ts          # generated from Go structs — never hand-edited
├── testdata/golden/          # shared fold fixtures, run by both Go and Vitest
├── infra/                    # Terraform
├── Makefile
└── docs/superpowers/specs/
```

### Go conventions

Idiomatic Go, not Java's shape. No `interfaces/` package, no `I`-prefixed names,
no DI framework, no global state, no `init()` magic. `main.go` constructs
everything explicitly.

- **Dependency inversion:** `match/service.go` declares the interfaces it needs
  (`type EventStore interface { Append(ctx, matchID string, e []core.Event) error }`)
  and imports nothing from `store/`. `store/firestore` satisfies them
  structurally without naming them. Dependencies point inward.
- **Interface segregation:** consumer-side interfaces stay at 1–3 methods. Many
  small interfaces, never one `Repository` with twenty methods.
- **Single responsibility:** enforced at the package boundary. `core` cannot
  reach the database because it does not import it.
- **Open/closed:** a new statistic is a new `EventKind` plus a case in a pure
  function. Nothing in `store/` or `httpapi/` changes.

### Web conventions

Feature folders, not file-type folders — everything for the tagging screen lives
in one directory.

**No state management library.** IndexedDB is the state. Dexie's `useLiveQuery`
subscribes components directly to local queries, so a tap that writes an event
re-renders every affected view automatically. Offline-first and state management
collapse into one mechanism instead of fighting each other. TanStack Query
appears only for genuinely server-only reads such as season analytics.

Tooling: Vite, TypeScript in `strict` mode, Vitest, Biome (lint + format),
Tailwind.

**Type safety across the boundary:** Go structs are the single source of truth.
`make types` generates `web/src/types/api.ts` from them, so renaming a Go field
without updating the client fails the TypeScript build rather than rendering
`undefined` during a match.

## 12. Testing

Test-driven throughout. Coverage is concentrated where being wrong is expensive.

| Layer | Approach |
|---|---|
| `core/` | Pure unit tests plus the shared golden fixtures. The bulk of the coverage. |
| `match/`, `season/` | Against a ~30-line in-memory store. No emulator. |
| `store/firestore/` | Firestore emulator. Thin layer, few tests. |
| `web/` | Vitest, including the same golden fixtures run against the TypeScript fold. |
| End-to-end | One Playwright test: tag a match with the network disabled, restore it, assert everything synced. |

## 13. Deployment

One container, one Cloud Run service, `min-instances=0`.

`make deploy` builds the web bundle, embeds it in the Go binary, pushes to
Artifact Registry, and deploys. GCP resources — Firestore, Cloud Run, Artifact
Registry, IAM — are defined in Terraform, so standing up a second environment is
`terraform apply` rather than archaeology.

GitHub Actions runs tests on every pull request and deploys on merge to `main`.

## 14. Build order

The scope above is larger than one implementation plan. It is built in
milestones, each independently useful:

- **M0 — Walking skeleton.** Go binary serving the PWA shell, Firebase Auth
  sign-in, Terraform, CI, deployed to Cloud Run. Proves the whole pipeline.
- **M1 — Squad and matches.** Club/team/player management, match creation,
  starting XV selection.
- **M2 — Live tagging.** The tagging screen, the event log, offline storage, the
  outbox and sync, the fold in both languages with golden fixtures, live counts.
  *This is the core value; everything before it is setup and everything after it
  is read-only.*
- **M3 — Match report.** The post-match report, time-slice breakdown, share
  links, print stylesheet.
- **M4 — Season view.** Player pages, trends, position-group comparison,
  per-player feedback cards.

The first implementation plan covers **M0 and M1**.

## 15. Out of scope for v1

Recorded explicitly so it is on the record rather than assumed:

video and clip tagging; availability and injury tracking; training-session data;
opposition scouting; GPS and wearable import; self-serve club signup; billing;
push notifications; native iOS or Android applications.

The multi-club **data model** is in from day one. Multi-club **onboarding** is not.
