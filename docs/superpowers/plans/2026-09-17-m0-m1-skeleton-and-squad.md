# M0 + M1: Walking Skeleton, Squad & Matches — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a deployed, authenticated Go + PWA application on Cloud Run, then add club squad management and match creation with starting-XV selection.

**Architecture:** A single Go binary serves both the JSON API and the embedded React bundle. Domain logic lives in `internal/core` with zero dependencies; services declare the small interfaces they need; `internal/store/firestore` satisfies them. The browser never touches Firestore.

**Tech Stack:** Go 1.24, `net/http` (Go 1.22+ `ServeMux` patterns — no third-party router), Firestore, Firebase Auth, React 19 + TypeScript + Vite, TanStack Query, Tailwind, Biome, Vitest, Terraform, GitHub Actions, Cloud Run.

**Spec:** `docs/superpowers/specs/2026-09-17-rugby-performance-tracker-design.md`

## Global Constraints

- Go module path: `pitch-ai`. Imports are `pitch-ai/internal/...`.
- Go 1.27 (the installed toolchain). TypeScript `strict: true`. Node 26.
- **`clubID` is always read from the caller's auth context, never from a request body or query parameter.** This is the multi-tenancy invariant; a reviewer should reject any handler that violates it.
- `internal/core` imports only the Go standard library. No Firestore, no HTTP, no logging.
- Interfaces are declared by the package that consumes them, never by the package that implements them. 1–3 methods each.
- No state management library in the web app. TanStack Query for server data; React state for local UI only.
- All money-free, all free-tier: `min-instances=0`, no Cloud SQL, no headless browser.
- **Alex runs `git commit` himself.** Every task's final step stages with `git add` and gives the commit message to use. Do not run `git commit`.

---

## File Structure

**M0 creates:**

| File | Responsibility |
|---|---|
| `go.mod`, `Makefile` | Module definition; build/test/deploy entry points |
| `cmd/server/main.go` | Process lifecycle and explicit dependency wiring. The only file that knows Firestore exists. |
| `internal/httpapi/router.go` | Route table only |
| `internal/httpapi/json.go` | `writeJSON`, `readJSON`, `writeError` helpers |
| `internal/httpapi/health.go` | Health endpoint |
| `internal/httpapi/spa.go` | Static asset serving with SPA fallback |
| `internal/auth/role.go` | `Role`, `Action`, permission table |
| `internal/auth/middleware.go` | Token verification, membership lookup, request context |
| `internal/auth/firebase.go` | Firebase `TokenVerifier` implementation |
| `internal/store/firestore/client.go` | Client construction, collection path helpers |
| `internal/store/firestore/membership.go` | `MembershipStore` implementation |
| `web/embed.go` | `go:embed` of the built bundle |
| `web/src/**` | Vite + React + TS application shell |
| `infra/*.tf` | Firestore, Artifact Registry, Cloud Run, IAM |
| `.github/workflows/ci.yml` | Test on PR, deploy on merge |

**M1 adds:**

| File | Responsibility |
|---|---|
| `internal/core/position.go` | Jersey number → position group. Pure. |
| `internal/core/player.go` | `Player` type and validation. Pure. |
| `internal/core/match.go` | `Match`, `Lineup` types and validation. Pure. |
| `internal/squad/service.go` | Player use-cases; declares `PlayerStore` |
| `internal/squad/memstore_test.go` | In-memory `PlayerStore` used by service tests |
| `internal/fixture/service.go` | Match use-cases; declares `MatchStore` |
| `internal/store/firestore/player.go` | `PlayerStore` implementation |
| `internal/store/firestore/match.go` | `MatchStore` implementation |
| `internal/httpapi/players.go` | Player handlers |
| `internal/httpapi/matches.go` | Match and lineup handlers |
| `web/src/lib/api.ts` | Typed fetch wrapper with auth header |
| `web/src/features/players/**` | Squad list and player form |
| `web/src/features/matches/**` | Match list, match form, lineup picker |

`internal/fixture` is named for a rugby fixture (a scheduled match), not for test fixtures.

---

## Task 1: Walking skeleton server

**Files:**
- Create: `go.mod`, `Makefile`, `cmd/server/main.go`, `internal/httpapi/router.go`, `internal/httpapi/json.go`, `internal/httpapi/health.go`
- Test: `internal/httpapi/health_test.go`

**Interfaces:**
- Produces: `httpapi.NewRouter(Deps) http.Handler`; `httpapi.Deps` struct (grows in later tasks); `httpapi.writeJSON(w http.ResponseWriter, status int, v any)`; `httpapi.writeError(w http.ResponseWriter, status int, msg string)`

- [ ] **Step 1: Initialise the module**

```bash
go mod init pitch-ai
mkdir -p cmd/server internal/httpapi
```

- [ ] **Step 2: Write the failing test**

Create `internal/httpapi/health_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(Deps{Logger: slog.New(slog.DiscardHandler)})

	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`status field = %q, want "ok"`, body["status"])
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/httpapi/`
Expected: FAIL — `undefined: NewRouter`, `undefined: Deps`.

- [ ] **Step 4: Write the JSON helpers**

Create `internal/httpapi/json.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	// The status line is already sent, so an encoding failure cannot be
	// reported to the client. Truncating the body is the only honest outcome.
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
```

- [ ] **Step 5: Write the router and health handler**

Create `internal/httpapi/router.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"
)

// Deps holds everything the HTTP layer needs. It is populated once, in main.
type Deps struct {
	Logger *slog.Logger
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", handleHealth)
	return mux
}
```

Create `internal/httpapi/health.go`:

```go
package httpapi

import "net/http"

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS — `TestHealthzReturnsOK`.

- [ ] **Step 7: Write main with graceful shutdown**

Create `cmd/server/main.go`:

```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pitch-ai/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.NewRouter(httpapi.Deps{Logger: logger}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutdown requested")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("listening", "port", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	<-shutdownDone
}
```

Cloud Run sends SIGTERM and expects the process to drain, so this is not optional ceremony.

- [ ] **Step 8: Write the Makefile**

Create `Makefile`:

```make
.PHONY: test test-go test-web build run fmt

test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && npm test -- --run

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

fmt:
	go fmt ./...
```

- [ ] **Step 9: Verify the binary runs**

Run: `make build && ./bin/server &` then `curl -s localhost:8080/api/healthz`
Expected: `{"status":"ok"}`. Stop the server with `kill %1`.

- [ ] **Step 10: Stage**

```bash
git add go.mod Makefile cmd internal
```
Commit message: `feat: walking skeleton HTTP server with health endpoint`

---

## Task 2: Web application shell, embedded in the binary

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/biome.json`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/index.css`, `web/embed.go`, `web/dist/.gitkeep`, `internal/httpapi/spa.go`
- Modify: `internal/httpapi/router.go`, `cmd/server/main.go`, `.gitignore`, `Makefile`
- Test: `internal/httpapi/spa_test.go`

**Interfaces:**
- Consumes: `httpapi.NewRouter`, `httpapi.Deps` (Task 1)
- Produces: `web.Assets() fs.FS`; `httpapi.Deps.Assets fs.FS` field; `httpapi.SPAHandler(assets fs.FS) http.Handler`

- [ ] **Step 1: Scaffold the web app**

```bash
npm create vite@latest web -- --template react-ts
cd web && npm install
npm install -D @biomejs/biome vitest @testing-library/react @testing-library/jest-dom jsdom tailwindcss @tailwindcss/vite
npx biome init
```

- [ ] **Step 2: Configure Vite, Tailwind and Vitest**

Replace `web/vite.config.ts`:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    globals: true,
  },
});
```

The dev proxy means `npm run dev` on port 5173 talks to the Go server on 8080 without CORS.

Replace `web/src/index.css` with a single line:

```css
@import "tailwindcss";
```

Set `"strict": true` in `web/tsconfig.json` under `compilerOptions` if the template has not already.

- [ ] **Step 3: Write the failing SPA-fallback test**

Create `internal/httpapi/spa_test.go`:

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><title>pitch-ai</title>")},
		"assets/app.js":    {Data: []byte("console.log(1)")},
	}
}

func TestSPAServesRealFile(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("body = %q, want the asset contents", rec.Body.String())
	}
}

func TestSPAFallsBackToIndexForClientRoutes(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/matches/abc123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>pitch-ai</title>") {
		t.Errorf("body = %q, want index.html", rec.Body.String())
	}
}

func TestSPAServesIndexAtRoot(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestSPA`
Expected: FAIL — `undefined: SPAHandler`.

- [ ] **Step 5: Implement the SPA handler**

Create `internal/httpapi/spa.go`:

```go
package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPAHandler serves static assets, falling back to index.html for any path
// that is not a real file so client-side routes survive a page reload.
func SPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		if _, err := fs.Stat(assets, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/httpapi/ -run TestSPA -v`
Expected: PASS — three tests.

- [ ] **Step 7: Wire assets into the router**

In `internal/httpapi/router.go`, add the field and the catch-all route:

```go
type Deps struct {
	Logger *slog.Logger
	Assets fs.FS
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", handleHealth)
	if d.Assets != nil {
		mux.Handle("GET /", SPAHandler(d.Assets))
	}
	return mux
}
```

Add `"io/fs"` to the imports. The `nil` guard is what keeps every existing handler test free of an asset bundle.

- [ ] **Step 8: Add the embed**

Create `web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built single-page application bundle.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// dist is embedded at compile time; a failure here is a build defect.
		panic(err)
	}
	return sub
}
```

Create the placeholder so the package compiles before the first web build:

```bash
mkdir -p web/dist && touch web/dist/.gitkeep
```

In `.gitignore`, add an exception directly below the `web/dist/` line:

```
web/dist/
!web/dist/.gitkeep
```

- [ ] **Step 9: Pass assets from main**

In `cmd/server/main.go`, import `"pitch-ai/web"` and change the handler line to:

```go
		Handler:           httpapi.NewRouter(httpapi.Deps{Logger: logger, Assets: web.Assets()}),
```

- [ ] **Step 10: Add web build steps to the Makefile**

Add to `Makefile`:

```make
.PHONY: web

web:
	cd web && npm ci && npm run build

build: web
	go build -o bin/server ./cmd/server
```

Delete the old standalone `build:` target so there is exactly one.

- [ ] **Step 11: Verify end to end**

Run: `make build && ./bin/server &` then `curl -s localhost:8080/ | head -c 100` and `curl -s localhost:8080/some/client/route | head -c 100`
Expected: both return the Vite `index.html`. `curl -s localhost:8080/api/healthz` still returns `{"status":"ok"}`. Stop with `kill %1`.

- [ ] **Step 12: Stage**

```bash
git add web internal/httpapi Makefile .gitignore cmd
```
Commit message: `feat: embed React PWA shell in the server binary`

---

## Task 3: Authentication middleware and roles

**Files:**
- Create: `internal/auth/role.go`, `internal/auth/membership.go`, `internal/auth/middleware.go`, `internal/httpapi/me.go`
- Modify: `internal/httpapi/router.go`
- Test: `internal/auth/role_test.go`, `internal/auth/middleware_test.go`

**Interfaces:**
- Consumes: `httpapi.Deps`, `httpapi.writeError` (Tasks 1–2)
- Produces:
  - `auth.Role` (`RoleCoach`, `RoleAnalyst`, `RoleAdmin`) with `Can(Action) bool`
  - `auth.Action` (`ActionRead`, `ActionTagMatch`, `ActionEditMatch`, `ActionManageSquad`)
  - `auth.Membership{UID, ClubID string; TeamIDs []string; Role Role}`
  - `auth.Claims{UID, Email string}`
  - `auth.TokenVerifier interface { Verify(ctx context.Context, idToken string) (Claims, error) }`
  - `auth.MembershipStore interface { Membership(ctx context.Context, uid string) (Membership, error) }`
  - `auth.ErrNoMembership`
  - `auth.Middleware(v TokenVerifier, s MembershipStore) func(http.Handler) http.Handler`
  - `auth.FromContext(ctx context.Context) (Membership, bool)`

- [ ] **Step 1: Write the failing permission test**

Create `internal/auth/role_test.go`:

```go
package auth

import "testing"

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role Role
		act  Action
		want bool
	}{
		{RoleCoach, ActionRead, true},
		{RoleCoach, ActionTagMatch, true},
		{RoleCoach, ActionManageSquad, false},
		{RoleCoach, ActionEditMatch, true},
		{RoleAnalyst, ActionRead, true},
		{RoleAnalyst, ActionEditMatch, true},
		{RoleAnalyst, ActionTagMatch, false},
		{RoleAnalyst, ActionManageSquad, false},
		{RoleAdmin, ActionManageSquad, true},
		{RoleAdmin, ActionTagMatch, true},
		{RoleAdmin, ActionEditMatch, true},
		{Role("nonsense"), ActionRead, false},
	}

	for _, c := range cases {
		if got := c.role.Can(c.act); got != c.want {
			t.Errorf("Role(%q).Can(%q) = %v, want %v", c.role, c.act, got, c.want)
		}
	}
}
```

The unknown-role case matters: an unrecognised value must deny everything rather than default open.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/auth/`
Expected: FAIL — `undefined: Role`.

- [ ] **Step 3: Implement roles**

Create `internal/auth/role.go`:

```go
package auth

type Role string

const (
	RoleCoach   Role = "coach"
	RoleAnalyst Role = "analyst"
	RoleAdmin   Role = "admin"
)

type Action string

const (
	ActionRead        Action = "read"
	ActionTagMatch    Action = "tag_match"
	ActionEditMatch   Action = "edit_match"
	ActionManageSquad Action = "manage_squad"
)

var rolePermissions = map[Role]map[Action]bool{
	RoleCoach: {
		ActionRead:      true,
		ActionTagMatch:  true,
		ActionEditMatch: true,
	},
	RoleAnalyst: {
		ActionRead:      true,
		ActionEditMatch: true,
	},
	RoleAdmin: {
		ActionRead:        true,
		ActionTagMatch:    true,
		ActionEditMatch:   true,
		ActionManageSquad: true,
	},
}

// Can reports whether the role permits the action. Unknown roles permit nothing.
func (r Role) Can(a Action) bool {
	return rolePermissions[r][a]
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/auth/ -run TestRolePermissions -v`
Expected: PASS.

- [ ] **Step 5: Write the failing middleware test**

Create `internal/auth/middleware_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeVerifier struct {
	claims Claims
	err    error
}

func (f fakeVerifier) Verify(context.Context, string) (Claims, error) {
	return f.claims, f.err
}

type fakeMemberships map[string]Membership

func (f fakeMemberships) Membership(_ context.Context, uid string) (Membership, error) {
	m, ok := f[uid]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return m, nil
}

func protectedHandler(t *testing.T, seen *Membership) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, ok := FromContext(r.Context())
		if !ok {
			t.Error("membership missing from request context")
		}
		*seen = m
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestMiddlewareAttachesMembership(t *testing.T) {
	want := Membership{UID: "uid-1", ClubID: "club-1", TeamIDs: []string{"team-1"}, Role: RoleAdmin}
	var got Membership

	mw := Middleware(
		fakeVerifier{claims: Claims{UID: "uid-1", Email: "coach@example.com"}},
		fakeMemberships{"uid-1": want},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer token-abc")
	rec := httptest.NewRecorder()
	mw(protectedHandler(t, &got)).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got.ClubID != want.ClubID || got.Role != want.Role {
		t.Errorf("membership = %+v, want %+v", got, want)
	}
}

func TestMiddlewareRejectsMissingHeader(t *testing.T) {
	mw := Middleware(fakeVerifier{}, fakeMemberships{})

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run without a token")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	mw := Middleware(fakeVerifier{err: errors.New("bad signature")}, fakeMemberships{})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer rubbish")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run for an invalid token")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejectsAuthenticatedNonMember(t *testing.T) {
	mw := Middleware(fakeVerifier{claims: Claims{UID: "stranger"}}, fakeMemberships{})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer token-abc")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run for a non-member")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
```

The last case is the important one: a valid Google account that belongs to no club is authenticated but not authorised — 403, not 401.

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test ./internal/auth/ -run TestMiddleware`
Expected: FAIL — `undefined: Middleware`, `undefined: Claims`, `undefined: ErrNoMembership`.

- [ ] **Step 7: Implement membership types**

Create `internal/auth/membership.go`:

```go
package auth

import (
	"context"
	"errors"
)

// ErrNoMembership means the token is valid but the account belongs to no club.
var ErrNoMembership = errors.New("auth: no membership for user")

type Claims struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
}

type Membership struct {
	UID     string   `json:"uid"`
	ClubID  string   `json:"clubId"`
	TeamIDs []string `json:"teamIds"`
	Role    Role     `json:"role"`
}

// TokenVerifier turns a bearer token into verified claims.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (Claims, error)
}

// MembershipStore resolves a verified user to their club and role.
type MembershipStore interface {
	Membership(ctx context.Context, uid string) (Membership, error)
}
```

- [ ] **Step 8: Implement the middleware**

Create `internal/auth/middleware.go`:

```go
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type contextKey struct{}

// FromContext returns the membership attached by Middleware.
func FromContext(ctx context.Context) (Membership, bool) {
	m, ok := ctx.Value(contextKey{}).(Membership)
	return m, ok
}

func Middleware(v TokenVerifier, s MembershipStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			claims, err := v.Verify(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			member, err := s.Membership(r.Context(), claims.UID)
			if err != nil {
				if errors.Is(err, ErrNoMembership) {
					http.Error(w, "account is not a member of any club", http.StatusForbidden)
					return
				}
				http.Error(w, "membership lookup failed", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), contextKey{}, member)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(header[len(prefix):]), true
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/auth/ -v`
Expected: PASS — five tests.

- [ ] **Step 10: Add the /api/me endpoint**

Create `internal/httpapi/me.go`:

```go
package httpapi

import (
	"net/http"

	"pitch-ai/internal/auth"
)

func handleMe(w http.ResponseWriter, r *http.Request) {
	member, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, member)
}
```

In `internal/httpapi/router.go`, add an `Auth func(http.Handler) http.Handler` field to `Deps` and register the route:

```go
	if d.Auth != nil {
		mux.Handle("GET /api/me", d.Auth(http.HandlerFunc(handleMe)))
	}
```

- [ ] **Step 11: Run the full Go suite**

Run: `go test ./...`
Expected: PASS, no failures.

- [ ] **Step 12: Stage**

```bash
git add internal/auth internal/httpapi
```
Commit message: `feat: Firebase token middleware, roles and /api/me`

---

## Task 4: Firestore client, membership store, emulator harness

**Files:**
- Create: `internal/store/firestore/client.go`, `internal/store/firestore/membership.go`, `internal/store/firestore/testing_test.go`, `internal/auth/firebase.go`, `scripts/emulator.sh`
- Modify: `cmd/server/main.go`, `Makefile`
- Test: `internal/store/firestore/membership_test.go`

**Interfaces:**
- Consumes: `auth.Membership`, `auth.MembershipStore`, `auth.ErrNoMembership`, `auth.TokenVerifier`, `auth.Claims` (Task 3)
- Produces:
  - `firestore.New(ctx context.Context, projectID string) (*Store, error)` returning `*Store` with `Close() error`
  - `(*Store).Membership(ctx context.Context, uid string) (auth.Membership, error)` — satisfies `auth.MembershipStore`
  - `(*Store).clubDoc(clubID string) *fs.DocumentRef` — unexported path helper used by later store files
  - `auth.NewFirebaseVerifier(ctx context.Context, projectID string) (auth.TokenVerifier, error)`
  - `firestore.newTestStore(t *testing.T) *Store` — skips when the emulator is absent

- [ ] **Step 1: Add dependencies**

```bash
go get cloud.google.com/go/firestore@latest
go get firebase.google.com/go/v4@latest
```

- [ ] **Step 2: Write the emulator script**

Create `scripts/emulator.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Firestore emulator for local tests. Requires: gcloud components install cloud-firestore-emulator
exec gcloud emulators firestore start --host-port=localhost:8081
```

```bash
chmod +x scripts/emulator.sh
```

Add to `Makefile`:

```make
.PHONY: emulator test-store

emulator:
	./scripts/emulator.sh

test-store:
	FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./internal/store/... -count=1
```

- [ ] **Step 3: Write the failing store test**

Create `internal/store/firestore/membership_test.go`:

```go
package firestore

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/auth"
)

func TestMembershipRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	want := auth.Membership{
		UID:     "uid-round-trip",
		ClubID:  "club-1",
		TeamIDs: []string{"team-1", "team-2"},
		Role:    auth.RoleCoach,
	}
	if _, err := store.client.Collection("users").Doc(want.UID).Set(ctx, want); err != nil {
		t.Fatalf("seeding membership: %v", err)
	}

	got, err := store.Membership(ctx, want.UID)
	if err != nil {
		t.Fatalf("Membership() error = %v", err)
	}
	if got.ClubID != want.ClubID || got.Role != want.Role || len(got.TeamIDs) != 2 {
		t.Errorf("Membership() = %+v, want %+v", got, want)
	}
}

func TestMembershipMissingUserIsErrNoMembership(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Membership(context.Background(), "uid-that-does-not-exist")
	if !errors.Is(err, auth.ErrNoMembership) {
		t.Errorf("error = %v, want auth.ErrNoMembership", err)
	}
}
```

Mapping Firestore's `NotFound` onto the domain's `ErrNoMembership` is the whole job of this layer — that translation is what keeps `internal/auth` ignorant of Firestore.

- [ ] **Step 4: Run the test to verify it fails**

Run: `FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./internal/store/firestore/`
Expected: FAIL — `undefined: newTestStore`.

- [ ] **Step 5: Write the test harness**

Create `internal/store/firestore/testing_test.go`:

```go
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
```

- [ ] **Step 6: Implement the client**

Create `internal/store/firestore/client.go`:

```go
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
```

- [ ] **Step 7: Implement the membership store**

Create `internal/store/firestore/membership.go`:

```go
package firestore

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pitch-ai/internal/auth"
)

func (s *Store) Membership(ctx context.Context, uid string) (auth.Membership, error) {
	snap, err := s.client.Collection("users").Doc(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return auth.Membership{}, auth.ErrNoMembership
	}
	if err != nil {
		return auth.Membership{}, fmt.Errorf("firestore: get membership %q: %w", uid, err)
	}

	var m auth.Membership
	if err := snap.DataTo(&m); err != nil {
		return auth.Membership{}, fmt.Errorf("firestore: decode membership %q: %w", uid, err)
	}
	m.UID = uid
	return m, nil
}
```

- [ ] **Step 8: Run the store tests against the emulator**

In one terminal: `make emulator`
In another: `make test-store`
Expected: PASS — both membership tests.

Then run `go test ./...` with no emulator variable set and confirm the store tests report `SKIP` rather than failing.

- [ ] **Step 9: Implement the Firebase verifier**

Create `internal/auth/firebase.go`:

```go
package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
)

type firebaseVerifier struct {
	client *firebaseauth.Client
}

// NewFirebaseVerifier verifies Firebase ID tokens against Google's public keys.
func NewFirebaseVerifier(ctx context.Context, projectID string) (TokenVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("auth: firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: firebase auth client: %w", err)
	}
	return firebaseVerifier{client: client}, nil
}

func (v firebaseVerifier) Verify(ctx context.Context, idToken string) (Claims, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return Claims{}, fmt.Errorf("auth: verify id token: %w", err)
	}
	email, _ := token.Claims["email"].(string)
	return Claims{UID: token.UID, Email: email}, nil
}
```

- [ ] **Step 10: Wire everything in main**

Replace the body of `main()` in `cmd/server/main.go` between the logger and the `srv` construction:

```go
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		logger.Error("GOOGLE_CLOUD_PROJECT is required")
		os.Exit(1)
	}

	store, err := firestorestore.New(ctx, projectID)
	if err != nil {
		logger.Error("connecting to firestore", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	verifier, err := auth.NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		logger.Error("building token verifier", "error", err)
		os.Exit(1)
	}
```

and set the handler to:

```go
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger: logger,
			Assets: web.Assets(),
			Auth:   auth.Middleware(verifier, store),
		}),
```

Import `"pitch-ai/internal/auth"` and `firestorestore "pitch-ai/internal/store/firestore"`. The import alias avoids colliding with the Google package name in any file that later needs both.

- [ ] **Step 11: Verify it builds and the suite is green**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests pass or skip.

- [ ] **Step 12: Stage**

```bash
git add internal/store internal/auth cmd scripts Makefile go.mod go.sum
```
Commit message: `feat: Firestore store, membership lookup and Firebase verifier`

---

## Task 5: Terraform and first Cloud Run deploy

**Files:**
- Create: `infra/main.tf`, `infra/variables.tf`, `infra/outputs.tf`, `infra/terraform.tfvars.example`, `Dockerfile`, `.dockerignore`
- Modify: `Makefile`, `.gitignore`

**Interfaces:**
- Consumes: the server binary from Task 1–4
- Produces: a deployed HTTPS URL; `make deploy`

- [ ] **Step 1: Write the Dockerfile**

Create `Dockerfile`:

```dockerfile
# syntax=docker/dockerfile:1

FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

Create `.dockerignore`:

```
.git
bin
web/node_modules
web/dist
infra
docs
.superpowers
```

`web/dist` is excluded so the stale local build never shadows the one produced in the `web` stage.

- [ ] **Step 2: Write the Terraform variables**

Create `infra/variables.tf`:

```hcl
variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "region" {
  type        = string
  description = "Region for Cloud Run and Artifact Registry"
  default     = "europe-west1"
}

variable "service_name" {
  type        = string
  description = "Cloud Run service name"
  default     = "pitch-ai"
}
```

Create `infra/terraform.tfvars.example`:

```hcl
project_id = "your-gcp-project-id"
region     = "europe-west1"
```

- [ ] **Step 3: Write the infrastructure**

Create `infra/main.tf`:

```hcl
terraform {
  required_version = ">= 1.9"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

resource "google_project_service" "required" {
  for_each = toset([
    "run.googleapis.com",
    "firestore.googleapis.com",
    "artifactregistry.googleapis.com",
    "identitytoolkit.googleapis.com",
  ])
  service            = each.value
  disable_on_destroy = false
}

resource "google_firestore_database" "main" {
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"
  depends_on  = [google_project_service.required]
}

resource "google_artifact_registry_repository" "containers" {
  repository_id = var.service_name
  location      = var.region
  format        = "DOCKER"
  depends_on    = [google_project_service.required]
}

resource "google_service_account" "server" {
  account_id   = "${var.service_name}-server"
  display_name = "pitch-ai Cloud Run service account"
}

resource "google_project_iam_member" "firestore_user" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.server.email}"
}

# Verifying Firebase ID tokens requires signing-key access.
resource "google_project_iam_member" "token_verifier" {
  project = var.project_id
  role    = "roles/firebaseauth.viewer"
  member  = "serviceAccount:${google_service_account.server.email}"
}

resource "google_cloud_run_v2_service" "server" {
  name                = var.service_name
  location            = var.region
  deletion_protection = false
  ingress             = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.server.email

    scaling {
      min_instance_count = 0
      max_instance_count = 4
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/${var.service_name}/${var.service_name}:latest"

      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      startup_probe {
        http_get {
          path = "/api/healthz"
        }
        initial_delay_seconds = 2
        period_seconds        = 3
        failure_threshold     = 10
      }
    }
  }

  lifecycle {
    # CI deploys new images; Terraform must not roll them back.
    ignore_changes = [template[0].containers[0].image]
  }

  depends_on = [google_project_service.required]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  name     = google_cloud_run_v2_service.server.name
  location = google_cloud_run_v2_service.server.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}
```

The service is publicly invokable because the PWA and its share links are opened by anonymous browsers — authorisation happens inside the Go middleware, not at the Cloud Run edge.

Create `infra/outputs.tf`:

```hcl
output "service_url" {
  value = google_cloud_run_v2_service.server.uri
}

output "image_repository" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${var.service_name}/${var.service_name}"
}
```

- [ ] **Step 4: Ignore Terraform state**

Add to `.gitignore`:

```
infra/.terraform/
infra/*.tfstate
infra/*.tfstate.*
infra/terraform.tfvars
```

- [ ] **Step 5: Apply the infrastructure**

```bash
cd infra
cp terraform.tfvars.example terraform.tfvars   # then edit project_id
terraform init
terraform apply
```

Expected: Firestore database, Artifact Registry repository, service account and Cloud Run service created. The first apply may fail on the Cloud Run resource because no image exists yet — that is expected; continue to the next step and re-apply.

- [ ] **Step 6: Add deploy targets**

Add to `Makefile`:

```make
.PHONY: deploy

REGION ?= europe-west1
SERVICE ?= pitch-ai
PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
IMAGE = $(REGION)-docker.pkg.dev/$(PROJECT)/$(SERVICE)/$(SERVICE)

deploy:
	gcloud builds submit --tag $(IMAGE):latest .
	gcloud run deploy $(SERVICE) --image $(IMAGE):latest --region $(REGION)
```

- [ ] **Step 7: Deploy and verify**

```bash
make deploy
curl -s "$(cd infra && terraform output -raw service_url)/api/healthz"
```

Expected: `{"status":"ok"}`. Open the service URL in a browser and confirm the Vite shell renders.

- [ ] **Step 8: Stage**

```bash
git add infra Dockerfile .dockerignore Makefile .gitignore
```
Commit message: `feat: Terraform infrastructure and Cloud Run deployment`

---

## Task 6: Continuous integration

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `make test-go`, `make test-web` (Task 1–2), the Dockerfile (Task 5)

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
      # web/dist is gitignored, so the embed directive needs a target to exist.
      - run: mkdir -p web/dist && touch web/dist/index.html
      - run: go vet ./...
      - run: go test ./... -race

  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "22"
          cache: npm
          cache-dependency-path: web/package-lock.json
      - run: npm ci
      - run: npx biome ci .
      - run: npx tsc --noEmit
      - run: npm test -- --run
      - run: npm run build
```

Deployment on merge is deliberately left out until the first manual deploy in Task 5 has been verified by hand; adding a deploy job before the pipeline is proven turns a broken build into a broken production.

- [ ] **Step 2: Verify the workflow locally**

Run: `go vet ./... && go test ./... -race && cd web && npx biome ci . && npx tsc --noEmit && npm run build`
Expected: all commands exit 0.

- [ ] **Step 3: Stage**

```bash
git add .github
```
Commit message: `ci: test Go and web on pull requests`

---

**M0 is complete at this point:** a deployed, authenticated binary serving the PWA shell, with infrastructure as code and CI.

---

## Task 7: Core player domain

**Files:**
- Create: `internal/core/errors.go`, `internal/core/position.go`, `internal/core/player.go`
- Test: `internal/core/position_test.go`, `internal/core/player_test.go`

**Interfaces:**
- Consumes: nothing. `internal/core` imports only the standard library.
- Produces:
  - `core.ErrNotFound`, `core.ValidationError` with `Error() string`
  - `core.PositionGroup` constants: `FrontRow`, `SecondRow`, `BackRow`, `HalfBacks`, `Centres`, `BackThree`
  - `core.PositionGroupForJersey(jersey int) (PositionGroup, error)`
  - `core.Player{ID, ClubID, FirstName, LastName, DOB string; Positions []PositionGroup; TeamIDs []string; Status PlayerStatus}`
  - `core.PlayerStatus` constants: `PlayerActive`, `PlayerInactive`
  - `(Player).Validate() error`, `(Player).DisplayName() string`

- [ ] **Step 1: Write the failing position test**

Create `internal/core/position_test.go`:

```go
package core

import "testing"

func TestPositionGroupForJersey(t *testing.T) {
	cases := map[int]PositionGroup{
		1: FrontRow, 2: FrontRow, 3: FrontRow,
		4: SecondRow, 5: SecondRow,
		6: BackRow, 7: BackRow, 8: BackRow,
		9: HalfBacks, 10: HalfBacks,
		11: BackThree, 14: BackThree, 15: BackThree,
		12: Centres, 13: Centres,
	}

	for jersey, want := range cases {
		got, err := PositionGroupForJersey(jersey)
		if err != nil {
			t.Errorf("PositionGroupForJersey(%d) error = %v", jersey, err)
			continue
		}
		if got != want {
			t.Errorf("PositionGroupForJersey(%d) = %q, want %q", jersey, got, want)
		}
	}
}

func TestPositionGroupForJerseyRejectsOutOfRange(t *testing.T) {
	for _, jersey := range []int{0, -1, 16, 23, 99} {
		if _, err := PositionGroupForJersey(jersey); err == nil {
			t.Errorf("PositionGroupForJersey(%d) error = nil, want an error", jersey)
		}
	}
}
```

Jerseys 11, 14 and 15 are the back three while 12 and 13 are centres — the numbering is not contiguous, which is exactly the kind of thing a lookup table gets wrong silently.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/`
Expected: FAIL — `undefined: PositionGroupForJersey`.

- [ ] **Step 3: Implement errors and positions**

Create `internal/core/errors.go`:

```go
// Package core holds the domain types and rules. It imports only the standard
// library: no storage, no transport, no logging.
package core

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned by stores when a requested entity does not exist.
var ErrNotFound = errors.New("core: not found")

// ValidationError describes a rejected field.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}
```

Create `internal/core/position.go`:

```go
package core

import "fmt"

type PositionGroup string

const (
	FrontRow  PositionGroup = "front_row"
	SecondRow PositionGroup = "second_row"
	BackRow   PositionGroup = "back_row"
	HalfBacks PositionGroup = "half_backs"
	Centres   PositionGroup = "centres"
	BackThree PositionGroup = "back_three"
)

// jerseyGroups maps a starting jersey number to its position group. Note that
// the back three (11, 14, 15) are not contiguous: 12 and 13 are the centres.
var jerseyGroups = map[int]PositionGroup{
	1: FrontRow, 2: FrontRow, 3: FrontRow,
	4: SecondRow, 5: SecondRow,
	6: BackRow, 7: BackRow, 8: BackRow,
	9: HalfBacks, 10: HalfBacks,
	11: BackThree,
	12: Centres, 13: Centres,
	14: BackThree, 15: BackThree,
}

// PositionGroupForJersey returns the position group for a starting jersey (1-15).
func PositionGroupForJersey(jersey int) (PositionGroup, error) {
	group, ok := jerseyGroups[jersey]
	if !ok {
		return "", fmt.Errorf("core: jersey %d is not a starting number (1-15)", jersey)
	}
	return group, nil
}

// Valid reports whether the position group is one of the known constants.
func (p PositionGroup) Valid() bool {
	switch p {
	case FrontRow, SecondRow, BackRow, HalfBacks, Centres, BackThree:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/ -run TestPosition -v`
Expected: PASS — two tests.

- [ ] **Step 5: Write the failing player test**

Create `internal/core/player_test.go`:

```go
package core

import (
	"strings"
	"testing"
)

func validPlayer() Player {
	return Player{
		ID:        "player-1",
		ClubID:    "club-1",
		FirstName: "Thomas",
		LastName:  "Lefevre",
		DOB:       "1998-04-12",
		Positions: []PositionGroup{BackRow},
		TeamIDs:   []string{"team-1"},
		Status:    PlayerActive,
	}
}

func TestPlayerValidateAcceptsAValidPlayer(t *testing.T) {
	if err := validPlayer().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestPlayerValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		mutate func(*Player)
		field string
	}{
		{"empty first name", func(p *Player) { p.FirstName = "  " }, "firstName"},
		{"empty last name", func(p *Player) { p.LastName = "" }, "lastName"},
		{"malformed dob", func(p *Player) { p.DOB = "12/04/1998" }, "dob"},
		{"impossible dob", func(p *Player) { p.DOB = "1998-13-45" }, "dob"},
		{"no positions", func(p *Player) { p.Positions = nil }, "positions"},
		{"unknown position", func(p *Player) { p.Positions = []PositionGroup{"prop"} }, "positions"},
		{"unknown status", func(p *Player) { p.Status = "retired-ish" }, "status"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validPlayer()
			c.mutate(&p)

			err := p.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestPlayerValidateAllowsEmptyDOB(t *testing.T) {
	p := validPlayer()
	p.DOB = ""

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil for an unrecorded date of birth", err)
	}
}

func TestPlayerDisplayName(t *testing.T) {
	if got, want := validPlayer().DisplayName(), "T. Lefevre"; got != want {
		t.Errorf("DisplayName() = %q, want %q", got, want)
	}
}
```

Date of birth is optional: senior squads often do not record it, and §10 of the spec keeps it out of anything shared. Requiring it would push coaches into entering fiction.

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test ./internal/core/ -run TestPlayer`
Expected: FAIL — `undefined: Player`.

- [ ] **Step 7: Implement the player**

Create `internal/core/player.go`:

```go
package core

import (
	"strings"
	"time"
)

type PlayerStatus string

const (
	PlayerActive   PlayerStatus = "active"
	PlayerInactive PlayerStatus = "inactive"
)

type Player struct {
	ID        string          `json:"id,omitempty" firestore:"-"`
	ClubID    string          `json:"clubId,omitempty" firestore:"-"`
	FirstName string          `json:"firstName" firestore:"firstName"`
	LastName  string          `json:"lastName" firestore:"lastName"`
	DOB       string          `json:"dob" firestore:"dob"`
	Positions []PositionGroup `json:"positions" firestore:"positions"`
	TeamIDs   []string        `json:"teamIds" firestore:"teamIds"`
	Status    PlayerStatus    `json:"status" firestore:"status"`
}

// DisplayName renders the shirt-back form: initial and surname.
func (p Player) DisplayName() string {
	first := strings.TrimSpace(p.FirstName)
	if first == "" {
		return strings.TrimSpace(p.LastName)
	}
	return string([]rune(first)[0]) + ". " + strings.TrimSpace(p.LastName)
}

// Validate checks the fields a caller supplies. ID and ClubID are assigned by
// the service layer and are deliberately not checked here.
func (p Player) Validate() error {
	if strings.TrimSpace(p.FirstName) == "" {
		return ValidationError{Field: "firstName", Reason: "must not be empty"}
	}
	if strings.TrimSpace(p.LastName) == "" {
		return ValidationError{Field: "lastName", Reason: "must not be empty"}
	}
	if p.DOB != "" {
		if _, err := time.Parse(time.DateOnly, p.DOB); err != nil {
			return ValidationError{Field: "dob", Reason: "must be a real date in YYYY-MM-DD form"}
		}
	}
	if len(p.Positions) == 0 {
		return ValidationError{Field: "positions", Reason: "at least one position group is required"}
	}
	for _, group := range p.Positions {
		if !group.Valid() {
			return ValidationError{Field: "positions", Reason: "unknown position group " + string(group)}
		}
	}
	switch p.Status {
	case PlayerActive, PlayerInactive:
	default:
		return ValidationError{Field: "status", Reason: "must be active or inactive"}
	}
	return nil
}
```

`ID` and `ClubID` carry `firestore:"-"` because the document ID and its path already encode them; storing them again invites the two copies to disagree.

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/core/ -v`
Expected: PASS — all position and player tests.

- [ ] **Step 9: Stage**

```bash
git add internal/core
```
Commit message: `feat(core): player domain, position groups and validation`

---

## Task 8: Squad service

**Files:**
- Create: `internal/squad/service.go`, `internal/squad/memstore_test.go`
- Test: `internal/squad/service_test.go`

**Interfaces:**
- Consumes: `core.Player`, `core.PlayerStatus`, `core.ErrNotFound`, `core.ValidationError` (Task 7)
- Produces:
  - `squad.PlayerReader interface { Player(ctx, clubID, playerID string) (core.Player, error); Players(ctx, clubID string) ([]core.Player, error) }`
  - `squad.PlayerWriter interface { PutPlayer(ctx, clubID string, p core.Player) error; DeletePlayer(ctx, clubID, playerID string) error }`
  - `squad.NewService(r PlayerReader, w PlayerWriter, newID func() string) *Service`
  - `(*Service).Add(ctx, clubID string, p core.Player) (core.Player, error)`
  - `(*Service).Update(ctx, clubID, playerID string, p core.Player) (core.Player, error)`
  - `(*Service).Get(ctx, clubID, playerID string) (core.Player, error)`
  - `(*Service).List(ctx, clubID string) ([]core.Player, error)`
  - `(*Service).Remove(ctx, clubID, playerID string) error`

- [ ] **Step 1: Write the in-memory store**

Create `internal/squad/memstore_test.go`:

```go
package squad

import (
	"context"
	"sort"

	"pitch-ai/internal/core"
)

// memStore is an in-memory PlayerReader + PlayerWriter for service tests.
// Keying by club is what lets the tests prove tenant isolation.
type memStore struct {
	players map[string]map[string]core.Player // clubID -> playerID -> player
}

func newMemStore() *memStore {
	return &memStore{players: map[string]map[string]core.Player{}}
}

func (m *memStore) Player(_ context.Context, clubID, playerID string) (core.Player, error) {
	p, ok := m.players[clubID][playerID]
	if !ok {
		return core.Player{}, core.ErrNotFound
	}
	return p, nil
}

func (m *memStore) Players(_ context.Context, clubID string) ([]core.Player, error) {
	out := make([]core.Player, 0, len(m.players[clubID]))
	for _, p := range m.players[clubID] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastName < out[j].LastName })
	return out, nil
}

func (m *memStore) PutPlayer(_ context.Context, clubID string, p core.Player) error {
	if m.players[clubID] == nil {
		m.players[clubID] = map[string]core.Player{}
	}
	m.players[clubID][p.ID] = p
	return nil
}

func (m *memStore) DeletePlayer(_ context.Context, clubID, playerID string) error {
	if _, ok := m.players[clubID][playerID]; !ok {
		return core.ErrNotFound
	}
	delete(m.players[clubID], playerID)
	return nil
}
```

- [ ] **Step 2: Write the failing service test**

Create `internal/squad/service_test.go`:

```go
package squad

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/core"
)

func samplePlayer() core.Player {
	return core.Player{
		FirstName: "Thomas",
		LastName:  "Lefevre",
		Positions: []core.PositionGroup{core.BackRow},
		Status:    core.PlayerActive,
	}
}

func TestAddAssignsIDAndClub(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	got, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.ID != "generated-id-a" {
		t.Errorf("ID = %q, want the generated id", got.ID)
	}
	if got.ClubID != "club-1" {
		t.Errorf("ClubID = %q, want club-1", got.ClubID)
	}
}

func TestAddIgnoresCallerSuppliedIDAndClub(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	p := samplePlayer()
	p.ID = "attacker-chosen-id"
	p.ClubID = "someone-elses-club"

	got, err := svc.Add(context.Background(), "club-1", p)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.ID != "generated-id-a" {
		t.Errorf("ID = %q, want the service-generated id", got.ID)
	}
	if got.ClubID != "club-1" {
		t.Errorf("ClubID = %q, want the caller's club, not the body's", got.ClubID)
	}
}

func TestAddRejectsInvalidPlayer(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	p := samplePlayer()
	p.LastName = ""

	if _, err := svc.Add(context.Background(), "club-1", p); err == nil {
		t.Fatal("Add() error = nil, want a validation error")
	}
}

func TestAddDefaultsStatusToActive(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	p := samplePlayer()
	p.Status = ""

	got, err := svc.Add(context.Background(), "club-1", p)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.Status != core.PlayerActive {
		t.Errorf("Status = %q, want %q", got.Status, core.PlayerActive)
	}
}

func TestGetDoesNotLeakAcrossClubs(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	added, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	_, err = svc.Get(context.Background(), "club-2", added.ID)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Get() from another club error = %v, want core.ErrNotFound", err)
	}
}

func TestUpdatePreservesIdentity(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	added, _ := svc.Add(context.Background(), "club-1", samplePlayer())

	changed := samplePlayer()
	changed.LastName = "Lefevre-Martin"
	changed.ID = "hijack"
	changed.ClubID = "club-2"

	got, err := svc.Update(context.Background(), "club-1", added.ID, changed)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.ID != added.ID || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (%q, club-1)", got.ID, got.ClubID, added.ID)
	}
	if got.LastName != "Lefevre-Martin" {
		t.Errorf("LastName = %q, want the updated value", got.LastName)
	}
}

func TestUpdateUnknownPlayerIsNotFound(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	_, err := svc.Update(context.Background(), "club-1", "nope", samplePlayer())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Update() error = %v, want core.ErrNotFound", err)
	}
}

func TestListReturnsOnlyTheCallersClub(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, store, func() string { return "generated-id-a" })

	_, _ = svc.Add(context.Background(), "club-1", samplePlayer())
	_, _ = svc.Add(context.Background(), "club-2", samplePlayer())

	list, err := svc.List(context.Background(), "club-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List()) = %d, want 1", len(list))
	}
}
```

`TestAddIgnoresCallerSuppliedIDAndClub` and `TestGetDoesNotLeakAcrossClubs` encode the tenancy invariant from Global Constraints. They must not be deleted or weakened.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/squad/`
Expected: FAIL — `undefined: NewService`, `undefined: Service`.

- [ ] **Step 4: Implement the service**

Create `internal/squad/service.go`:

```go
// Package squad holds the player-management use-cases. It declares the storage
// interfaces it needs and knows nothing about Firestore or HTTP.
package squad

import (
	"context"
	"fmt"

	"pitch-ai/internal/core"
)

type PlayerReader interface {
	Player(ctx context.Context, clubID, playerID string) (core.Player, error)
	Players(ctx context.Context, clubID string) ([]core.Player, error)
}

type PlayerWriter interface {
	PutPlayer(ctx context.Context, clubID string, p core.Player) error
	DeletePlayer(ctx context.Context, clubID, playerID string) error
}

type Service struct {
	reader PlayerReader
	writer PlayerWriter
	newID  func() string
}

func NewService(r PlayerReader, w PlayerWriter, newID func() string) *Service {
	return &Service{reader: r, writer: w, newID: newID}
}

// Add stores a new player. The caller's clubID always wins over anything in p.
func (s *Service) Add(ctx context.Context, clubID string, p core.Player) (core.Player, error) {
	p.ID = s.newID()
	p.ClubID = clubID
	if p.Status == "" {
		p.Status = core.PlayerActive
	}

	if err := p.Validate(); err != nil {
		return core.Player{}, err
	}
	if err := s.writer.PutPlayer(ctx, clubID, p); err != nil {
		return core.Player{}, fmt.Errorf("squad: add player: %w", err)
	}
	return p, nil
}

// Update replaces a player's mutable fields. Identity comes from the path, never
// from the request body.
func (s *Service) Update(ctx context.Context, clubID, playerID string, p core.Player) (core.Player, error) {
	if _, err := s.reader.Player(ctx, clubID, playerID); err != nil {
		return core.Player{}, err
	}

	p.ID = playerID
	p.ClubID = clubID
	if p.Status == "" {
		p.Status = core.PlayerActive
	}

	if err := p.Validate(); err != nil {
		return core.Player{}, err
	}
	if err := s.writer.PutPlayer(ctx, clubID, p); err != nil {
		return core.Player{}, fmt.Errorf("squad: update player %q: %w", playerID, err)
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, clubID, playerID string) (core.Player, error) {
	return s.reader.Player(ctx, clubID, playerID)
}

func (s *Service) List(ctx context.Context, clubID string) ([]core.Player, error) {
	return s.reader.Players(ctx, clubID)
}

func (s *Service) Remove(ctx context.Context, clubID, playerID string) error {
	return s.writer.DeletePlayer(ctx, clubID, playerID)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/squad/ -v`
Expected: PASS — eight tests.

- [ ] **Step 6: Stage**

```bash
git add internal/squad
```
Commit message: `feat(squad): player service with club-scoped access`

---

## Task 9: Firestore player store

**Files:**
- Create: `internal/store/firestore/player.go`
- Test: `internal/store/firestore/player_test.go`

**Interfaces:**
- Consumes: `(*Store).clubDoc` (Task 4), `squad.PlayerReader`, `squad.PlayerWriter` (Task 8), `core.Player`, `core.ErrNotFound` (Task 7), `newTestStore` (Task 4)
- Produces: `(*Store).Player`, `(*Store).Players`, `(*Store).PutPlayer`, `(*Store).DeletePlayer` — together satisfying both squad interfaces

- [ ] **Step 1: Write the failing test**

Create `internal/store/firestore/player_test.go`:

```go
package firestore

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/core"
)

func seedPlayer(t *testing.T, store *Store, clubID, id, lastName string) core.Player {
	t.Helper()
	p := core.Player{
		ID:        id,
		ClubID:    clubID,
		FirstName: "Thomas",
		LastName:  lastName,
		Positions: []core.PositionGroup{core.BackRow},
		TeamIDs:   []string{"team-1"},
		Status:    core.PlayerActive,
	}
	if err := store.PutPlayer(context.Background(), clubID, p); err != nil {
		t.Fatalf("PutPlayer() error = %v", err)
	}
	return p
}

func TestPlayerRoundTrip(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-player-round-trip"

	want := seedPlayer(t, store, clubID, "player-1", "Lefevre")

	got, err := store.Player(context.Background(), clubID, "player-1")
	if err != nil {
		t.Fatalf("Player() error = %v", err)
	}
	if got.LastName != want.LastName || got.ID != "player-1" || got.ClubID != clubID {
		t.Errorf("Player() = %+v, want %+v", got, want)
	}
	if len(got.Positions) != 1 || got.Positions[0] != core.BackRow {
		t.Errorf("Positions = %v, want [back_row]", got.Positions)
	}
}

func TestPlayerMissingIsErrNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Player(context.Background(), "club-missing", "no-such-player")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Player() error = %v, want core.ErrNotFound", err)
	}
}

func TestPlayersAreScopedToTheirClub(t *testing.T) {
	store := newTestStore(t)

	seedPlayer(t, store, "club-scope-a", "p1", "Alpha")
	seedPlayer(t, store, "club-scope-a", "p2", "Bravo")
	seedPlayer(t, store, "club-scope-b", "p3", "Charlie")

	list, err := store.Players(context.Background(), "club-scope-a")
	if err != nil {
		t.Fatalf("Players() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(Players()) = %d, want 2", len(list))
	}
	if list[0].LastName != "Alpha" || list[1].LastName != "Bravo" {
		t.Errorf("Players() = [%q %q], want sorted [Alpha Bravo]", list[0].LastName, list[1].LastName)
	}
}

func TestDeletePlayerRemovesIt(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-delete"
	seedPlayer(t, store, clubID, "p-del", "Delete")

	if err := store.DeletePlayer(context.Background(), clubID, "p-del"); err != nil {
		t.Fatalf("DeletePlayer() error = %v", err)
	}
	if _, err := store.Player(context.Background(), clubID, "p-del"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Player() after delete error = %v, want core.ErrNotFound", err)
	}
}
```

Each test uses its own `clubID` so the emulator's shared state cannot make one test's data visible to another.

- [ ] **Step 2: Run the test to verify it fails**

Run: `make emulator` in one terminal, then `make test-store`
Expected: FAIL — `store.PutPlayer undefined`.

- [ ] **Step 3: Implement the store**

Create `internal/store/firestore/player.go`:

```go
package firestore

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pitch-ai/internal/core"
)

func (s *Store) playersCol(clubID string) *fs.CollectionRef {
	return s.clubDoc(clubID).Collection("players")
}

func (s *Store) Player(ctx context.Context, clubID, playerID string) (core.Player, error) {
	snap, err := s.playersCol(clubID).Doc(playerID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return core.Player{}, core.ErrNotFound
	}
	if err != nil {
		return core.Player{}, fmt.Errorf("firestore: get player %q: %w", playerID, err)
	}
	return playerFromSnap(snap, clubID)
}

func (s *Store) Players(ctx context.Context, clubID string) ([]core.Player, error) {
	iter := s.playersCol(clubID).OrderBy("lastName", fs.Asc).Documents(ctx)
	defer iter.Stop()

	var out []core.Player
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore: list players: %w", err)
		}
		p, err := playerFromSnap(snap, clubID)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
}

func (s *Store) PutPlayer(ctx context.Context, clubID string, p core.Player) error {
	if _, err := s.playersCol(clubID).Doc(p.ID).Set(ctx, p); err != nil {
		return fmt.Errorf("firestore: put player %q: %w", p.ID, err)
	}
	return nil
}

func (s *Store) DeletePlayer(ctx context.Context, clubID, playerID string) error {
	if _, err := s.playersCol(clubID).Doc(playerID).Delete(ctx); err != nil {
		return fmt.Errorf("firestore: delete player %q: %w", playerID, err)
	}
	return nil
}

// playerFromSnap decodes a document and restores the identity fields, which are
// carried by the document path rather than stored in the document body.
func playerFromSnap(snap *fs.DocumentSnapshot, clubID string) (core.Player, error) {
	var p core.Player
	if err := snap.DataTo(&p); err != nil {
		return core.Player{}, fmt.Errorf("firestore: decode player %q: %w", snap.Ref.ID, err)
	}
	p.ID = snap.Ref.ID
	p.ClubID = clubID
	return p, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-store`
Expected: PASS — membership and player tests.

- [ ] **Step 5: Stage**

```bash
git add internal/store/firestore go.mod go.sum
```
Commit message: `feat(store): Firestore player collection`

---

## Task 10: Player HTTP handlers and generated TypeScript types

**Files:**
- Create: `internal/httpapi/players.go`, `internal/httpapi/errors.go`, `internal/httpapi/testing_test.go`, `tygo.yaml`
- Modify: `internal/auth/middleware.go`, `internal/httpapi/router.go`, `cmd/server/main.go`, `Makefile`, `.github/workflows/ci.yml`
- Test: `internal/httpapi/players_test.go`

**Interfaces:**
- Consumes: `squad.Service` (Task 8), `auth.Membership`, `auth.Role`, `auth.Action` (Task 3), `core.ErrNotFound`, `core.ValidationError` (Task 7)
- Produces:
  - `auth.NewContext(ctx context.Context, m Membership) context.Context`
  - `httpapi.Deps.Squad *squad.Service` field
  - Routes: `GET|POST /api/players`, `GET|PUT|DELETE /api/players/{playerID}`
  - `web/src/types/core.ts` and `web/src/types/auth.ts`, generated

- [ ] **Step 1: Export a context constructor for tests and handlers**

Add to `internal/auth/middleware.go`, directly below `FromContext`:

```go
// NewContext attaches a membership to a context. Middleware uses it in
// production; handler tests use it to build an authenticated request.
func NewContext(ctx context.Context, m Membership) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}
```

Then replace the `context.WithValue` line inside `Middleware` with `ctx := NewContext(r.Context(), member)` so there is one way to do it.

Run: `go test ./internal/auth/`
Expected: PASS — unchanged behaviour.

- [ ] **Step 2: Write the handler test harness**

Create `internal/httpapi/testing_test.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/squad"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// fakeAuthMiddleware injects a membership directly, so handler tests exercise
// routing and permissions without minting real Firebase tokens.
func fakeAuthMiddleware(member auth.Membership) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), member)))
		})
	}
}

// newTestRouter builds a router wired to a squad service and a fake identity.
func newTestRouter(t *testing.T, member auth.Membership, svc *squad.Service) http.Handler {
	t.Helper()

	return NewRouter(Deps{
		Logger: discardLogger(),
		Squad:  svc,
		Auth:   fakeAuthMiddleware(member),
	})
}

func do(t *testing.T, router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
```

`discardLogger` and `fakeAuthMiddleware` are shared with the match handler tests in Task 14, which is why they are named helpers rather than inline literals.

- [ ] **Step 3: Write the failing handler test**

Create `internal/httpapi/players_test.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
	"pitch-ai/internal/squad"
)

type fakeStore struct {
	players map[string]map[string]core.Player
}

func newFakeStore() *fakeStore {
	return &fakeStore{players: map[string]map[string]core.Player{}}
}

func (f *fakeStore) Player(_ context.Context, clubID, playerID string) (core.Player, error) {
	p, ok := f.players[clubID][playerID]
	if !ok {
		return core.Player{}, core.ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) Players(_ context.Context, clubID string) ([]core.Player, error) {
	out := make([]core.Player, 0, len(f.players[clubID]))
	for _, p := range f.players[clubID] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastName < out[j].LastName })
	return out, nil
}

func (f *fakeStore) PutPlayer(_ context.Context, clubID string, p core.Player) error {
	if f.players[clubID] == nil {
		f.players[clubID] = map[string]core.Player{}
	}
	f.players[clubID][p.ID] = p
	return nil
}

func (f *fakeStore) DeletePlayer(_ context.Context, clubID, playerID string) error {
	if _, ok := f.players[clubID][playerID]; !ok {
		return core.ErrNotFound
	}
	delete(f.players[clubID], playerID)
	return nil
}

func adminRouter(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	svc := squad.NewService(store, store, func() string { return "player-generated" })
	member := auth.Membership{UID: "uid-1", ClubID: "club-1", Role: auth.RoleAdmin}
	return newTestRouter(t, member, svc), store
}

const validPlayerJSON = `{"firstName":"Thomas","lastName":"Lefevre","dob":"1998-04-12","positions":["back_row"],"teamIds":["team-1"],"status":"active"}`

func TestCreatePlayer(t *testing.T) {
	router, store := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players", validPlayerJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var got core.Player
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != "player-generated" || got.ClubID != "club-1" {
		t.Errorf("player = (%q, %q), want (player-generated, club-1)", got.ID, got.ClubID)
	}
	if _, ok := store.players["club-1"]["player-generated"]; !ok {
		t.Error("player was not persisted under the caller's club")
	}
}

func TestCreatePlayerRejectsInvalidBody(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players", `{"firstName":"Thomas","lastName":"","positions":["back_row"],"status":"active"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreatePlayerIgnoresBodyClubID(t *testing.T) {
	router, store := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players",
		`{"firstName":"T","lastName":"L","positions":["back_row"],"status":"active","clubId":"club-2"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if _, ok := store.players["club-2"]; ok {
		t.Error("player was written to the club named in the body, not the caller's club")
	}
	if _, ok := store.players["club-1"]["player-generated"]; !ok {
		t.Error("player was not written to the caller's club")
	}
}

func TestCreatePlayerRejectsUnknownFields(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players",
		`{"firstName":"T","lastName":"L","positions":["back_row"],"status":"active","nickname":"Tommo"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unrecognised field", rec.Code)
	}
}

func TestListPlayers(t *testing.T) {
	router, _ := adminRouter(t)
	_ = do(t, router, http.MethodPost, "/api/players", validPlayerJSON)

	rec := do(t, router, http.MethodGet, "/api/players", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []core.Player
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestGetUnknownPlayerIs404(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodGet, "/api/players/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeletePlayer(t *testing.T) {
	router, _ := adminRouter(t)
	_ = do(t, router, http.MethodPost, "/api/players", validPlayerJSON)

	rec := do(t, router, http.MethodDelete, "/api/players/player-generated", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	if got := do(t, router, http.MethodGet, "/api/players/player-generated", ""); got.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want 404", got.Code)
	}
}

func TestCoachCannotManageSquad(t *testing.T) {
	store := newFakeStore()
	svc := squad.NewService(store, store, func() string { return "player-generated" })
	member := auth.Membership{UID: "uid-2", ClubID: "club-1", Role: auth.RoleCoach}
	router := newTestRouter(t, member, svc)

	if rec := do(t, router, http.MethodPost, "/api/players", validPlayerJSON); rec.Code != http.StatusForbidden {
		t.Errorf("POST status = %d, want 403", rec.Code)
	}
	if rec := do(t, router, http.MethodGet, "/api/players", ""); rec.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200 — a coach may read the squad", rec.Code)
	}
}
```

`TestCreatePlayerIgnoresBodyClubID` is the HTTP-layer half of the tenancy invariant from Global Constraints: a body may name a club, and it must have no effect. `TestCreatePlayerRejectsUnknownFields` pins `DisallowUnknownFields` so a typo in a field name fails loudly instead of being silently dropped.

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestCreatePlayer`
Expected: FAIL — `Deps.Squad undefined`.

- [ ] **Step 5: Write the error mapper**

Create `internal/httpapi/errors.go`:

```go
package httpapi

import (
	"errors"
	"net/http"

	"pitch-ai/internal/core"
)

// writeDomainError maps domain errors onto status codes. Anything unrecognised
// is a 500 with the detail logged rather than returned.
func (d Deps) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid core.ValidationError
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Error())
	default:
		d.Logger.Error("unhandled error", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
```

- [ ] **Step 6: Write the player handlers**

Create `internal/httpapi/players.go`:

```go
package httpapi

import (
	"net/http"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

func (d Deps) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	players, err := d.Squad.List(r.Context(), member.ClubID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	if players == nil {
		players = []core.Player{}
	}
	writeJSON(w, http.StatusOK, players)
}

func (d Deps) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	player, err := d.Squad.Get(r.Context(), member.ClubID, r.PathValue("playerID"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, player)
}

func (d Deps) handleCreatePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	var body core.Player
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	created, err := d.Squad.Add(r.Context(), member.ClubID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (d Deps) handleUpdatePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	var body core.Player
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	updated, err := d.Squad.Update(r.Context(), member.ClubID, r.PathValue("playerID"), body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (d Deps) handleDeletePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	if err := d.Squad.Remove(r.Context(), member.ClubID, r.PathValue("playerID")); err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`core.Player` carries `json:"id,omitempty"` and `json:"clubId,omitempty"`, so a client may send them and the service overwrites both — which is exactly what `TestCreatePlayerIgnoresBodyClubID` asserts. Any field the struct does not declare is rejected by `DisallowUnknownFields`.

- [ ] **Step 7: Register the routes**

In `internal/httpapi/router.go`, add the `Squad` field and the permission helper, then the routes:

```go
type Deps struct {
	Logger *slog.Logger
	Assets fs.FS
	Auth   func(http.Handler) http.Handler
	Squad  *squad.Service
}

func (d Deps) protected(action auth.Action, h http.HandlerFunc) http.Handler {
	return d.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.FromContext(r.Context())
		if !ok || !member.Role.Can(action) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		h(w, r)
	}))
}
```

and inside `NewRouter`, below the `/api/me` block:

```go
	if d.Auth != nil && d.Squad != nil {
		mux.Handle("GET /api/players", d.protected(auth.ActionRead, d.handleListPlayers))
		mux.Handle("GET /api/players/{playerID}", d.protected(auth.ActionRead, d.handleGetPlayer))
		mux.Handle("POST /api/players", d.protected(auth.ActionManageSquad, d.handleCreatePlayer))
		mux.Handle("PUT /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleUpdatePlayer))
		mux.Handle("DELETE /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleDeletePlayer))
	}
```

Add `"pitch-ai/internal/auth"` and `"pitch-ai/internal/squad"` to the imports.

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS — health, SPA and all player tests.

- [ ] **Step 9: Wire the service in main**

In `cmd/server/main.go`, after the store is constructed:

```go
	squadService := squad.NewService(store, store, func() string { return uuid.NewString() })
```

and add `Squad: squadService` to the `httpapi.Deps` literal. Then:

```bash
go get github.com/google/uuid@latest
```

Add the imports `"github.com/google/uuid"` and `"pitch-ai/internal/squad"`.

- [ ] **Step 10: Configure type generation**

Create `tygo.yaml`:

```yaml
packages:
  - path: "pitch-ai/internal/core"
    output_path: "web/src/types/core.ts"
    type_mappings:
      time.Time: "string"
  - path: "pitch-ai/internal/auth"
    output_path: "web/src/types/auth.ts"
```

Add to `Makefile`:

```make
.PHONY: types

types:
	go run github.com/gzuidhof/tygo@v0.2.17 generate
```

- [ ] **Step 11: Generate and inspect the types**

Run: `mkdir -p web/src/types && make types && cat web/src/types/core.ts`
Expected: TypeScript interfaces for `Player`, plus the `PositionGroup` and `PlayerStatus` string unions.

- [ ] **Step 12: Fail CI on stale types**

In `.github/workflows/ci.yml`, add to the end of the `go` job's steps:

```yaml
      - run: make types
      - name: Fail if generated types are stale
        run: git diff --exit-code web/src/types
```

- [ ] **Step 13: Verify the full suite**

Run: `go build ./... && go test ./... && make types && git diff --exit-code web/src/types`
Expected: all green, no diff.

- [ ] **Step 14: Stage**

```bash
git add internal cmd tygo.yaml Makefile .github web/src/types go.mod go.sum
```
Commit message: `feat(api): player endpoints with role checks and generated types`

---

## Task 11: Web authentication shell and API client

**Files:**
- Create: `web/.env.example`, `web/src/lib/firebase.ts`, `web/src/lib/api.ts`, `web/src/lib/auth.tsx`, `web/src/AppShell.tsx`
- Modify: `web/src/main.tsx`, `web/src/App.tsx`, `.gitignore`
- Test: `web/src/lib/api.test.ts`

**Interfaces:**
- Consumes: `GET /api/me` (Task 3), `web/src/types/auth.ts` (Task 10)
- Produces:
  - `ApiError` class with `status: number`
  - `createApiClient(getToken: () => Promise<string | null>): ApiFetch`
  - `ApiFetch = <T>(path: string, init?: RequestInit) => Promise<T>`
  - `<AuthProvider>` and `useAuth(): { user, membership, apiFetch, signIn, signOut, loading }`

- [ ] **Step 1: Install dependencies**

```bash
cd web
npm install firebase @tanstack/react-query react-router
```

- [ ] **Step 2: Write the failing API client test**

Create `web/src/lib/api.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./api";

function mockFetch(response: Response) {
  const spy = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createApiClient", () => {
  it("prefixes /api and attaches the bearer token", async () => {
    const spy = mockFetch(
      new Response(JSON.stringify({ id: "p1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const apiFetch = createApiClient(async () => "token-abc");

    const result = await apiFetch<{ id: string }>("/players");

    expect(result).toEqual({ id: "p1" });
    const [url, init] = spy.mock.calls[0];
    expect(url).toBe("/api/players");
    expect(new Headers(init.headers).get("Authorization")).toBe("Bearer token-abc");
  });

  it("omits the header when there is no token", async () => {
    const spy = mockFetch(new Response("{}", { status: 200 }));
    const apiFetch = createApiClient(async () => null);

    await apiFetch("/players");

    const [, init] = spy.mock.calls[0];
    expect(new Headers(init.headers).get("Authorization")).toBeNull();
  });

  it("returns undefined for 204 responses", async () => {
    mockFetch(new Response(null, { status: 204 }));
    const apiFetch = createApiClient(async () => "token-abc");

    await expect(apiFetch("/players/p1", { method: "DELETE" })).resolves.toBeUndefined();
  });

  it("throws ApiError carrying the status and server message", async () => {
    mockFetch(
      new Response(JSON.stringify({ error: "lastName: must not be empty" }), { status: 400 }),
    );
    const apiFetch = createApiClient(async () => "token-abc");

    await expect(apiFetch("/players", { method: "POST", body: "{}" })).rejects.toMatchObject({
      status: 400,
      message: "lastName: must not be empty",
    });
    await expect(apiFetch("/players", { method: "POST", body: "{}" })).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd web && npm test -- --run src/lib/api.test.ts`
Expected: FAIL — cannot resolve `./api`.

- [ ] **Step 4: Implement the API client**

Create `web/src/lib/api.ts`:

```ts
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export type ApiFetch = <T>(path: string, init?: RequestInit) => Promise<T>;

/**
 * Builds a fetch wrapper that talks to the Go API. Every request carries the
 * caller's Firebase ID token; the server resolves the club from it, so no
 * request ever names a club itself.
 */
export function createApiClient(getToken: () => Promise<string | null>): ApiFetch {
  return async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);

    const token = await getToken();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    if (init.body !== undefined && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(`/api${path}`, { ...init, headers });

    if (response.status === 204) {
      return undefined as T;
    }

    const text = await response.text();
    const body = text ? JSON.parse(text) : null;

    if (!response.ok) {
      throw new ApiError(response.status, body?.error ?? response.statusText);
    }
    return body as T;
  };
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd web && npm test -- --run src/lib/api.test.ts`
Expected: PASS — four tests.

- [ ] **Step 6: Configure Firebase**

Create `web/.env.example`:

```
VITE_FIREBASE_API_KEY=replace-me
VITE_FIREBASE_AUTH_DOMAIN=your-project.firebaseapp.com
VITE_FIREBASE_PROJECT_ID=your-project
```

Copy it to `web/.env.local` and fill in the values from the Firebase console (Project settings → Your apps → Web app). Add to `.gitignore`:

```
web/.env.local
```

Create `web/src/lib/firebase.ts`:

```ts
import { initializeApp } from "firebase/app";
import { GoogleAuthProvider, getAuth } from "firebase/auth";

export const firebaseApp = initializeApp({
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
});

export const firebaseAuth = getAuth(firebaseApp);
export const googleProvider = new GoogleAuthProvider();
```

- [ ] **Step 7: Write the auth provider**

Create `web/src/lib/auth.tsx`:

```tsx
import { type User, onAuthStateChanged, signInWithPopup, signOut } from "firebase/auth";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { Membership } from "../types/auth";
import { type ApiFetch, createApiClient } from "./api";
import { firebaseAuth, googleProvider } from "./firebase";

type AuthState = {
  user: User | null;
  membership: Membership | null;
  loading: boolean;
  error: string | null;
  apiFetch: ApiFetch;
  signIn: () => Promise<void>;
  signOutUser: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [membership, setMembership] = useState<Membership | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const apiFetch = useMemo(
    () => createApiClient(() => firebaseAuth.currentUser?.getIdToken() ?? Promise.resolve(null)),
    [],
  );

  useEffect(() => {
    return onAuthStateChanged(firebaseAuth, async (next) => {
      setUser(next);
      setError(null);

      if (!next) {
        setMembership(null);
        setLoading(false);
        return;
      }

      try {
        setMembership(await apiFetch<Membership>("/me"));
      } catch {
        // A valid Google account that belongs to no club lands here.
        setMembership(null);
        setError("This account is not a member of any club yet.");
      } finally {
        setLoading(false);
      }
    });
  }, [apiFetch]);

  const value: AuthState = {
    user,
    membership,
    loading,
    error,
    apiFetch,
    signIn: async () => {
      await signInWithPopup(firebaseAuth, googleProvider);
    },
    signOutUser: async () => {
      await signOut(firebaseAuth);
    },
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return ctx;
}
```

- [ ] **Step 8: Write the shell**

Create `web/src/AppShell.tsx`:

```tsx
import { NavLink, Outlet } from "react-router";
import { useAuth } from "./lib/auth";

export function AppShell() {
  const { user, membership, loading, error, signIn, signOutUser } = useAuth();

  if (loading) {
    return <p className="p-8 text-slate-500">Loading…</p>;
  }

  if (!user) {
    return (
      <main className="grid min-h-dvh place-items-center bg-slate-50">
        <div className="space-y-4 text-center">
          <h1 className="text-2xl font-bold">pitch-ai</h1>
          <button
            type="button"
            onClick={signIn}
            className="rounded-lg bg-slate-900 px-5 py-3 font-semibold text-white"
          >
            Sign in with Google
          </button>
        </div>
      </main>
    );
  }

  if (!membership) {
    return <p className="p-8 text-red-700">{error ?? "No club membership."}</p>;
  }

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    isActive ? "font-semibold text-slate-900" : "text-slate-500";

  return (
    <div className="min-h-dvh bg-slate-50">
      <header className="flex items-center gap-6 border-b border-slate-200 bg-white px-6 py-3">
        <span className="font-bold">pitch-ai</span>
        <nav className="flex gap-4">
          <NavLink to="/squad" className={linkClass}>
            Squad
          </NavLink>
          <NavLink to="/matches" className={linkClass}>
            Matches
          </NavLink>
        </nav>
        <button type="button" onClick={signOutUser} className="ml-auto text-sm text-slate-500">
          Sign out
        </button>
      </header>
      <main className="mx-auto max-w-5xl p-6">
        <Outlet />
      </main>
    </div>
  );
}
```

- [ ] **Step 9: Wire providers and routes**

Replace `web/src/main.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Navigate, RouterProvider, createBrowserRouter } from "react-router";
import { AppShell } from "./AppShell";
import "./index.css";
import { AuthProvider } from "./lib/auth";

const queryClient = new QueryClient();

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [{ index: true, element: <Navigate to="/squad" replace /> }],
  },
]);

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  </StrictMode>,
);
```

Delete `web/src/App.tsx` and its stylesheet if the Vite template created one — `AppShell` replaces it.

- [ ] **Step 10: Verify sign-in end to end**

Enable Google sign-in in the Firebase console (Authentication → Sign-in method), and add `localhost` to the authorised domains.

Seed your own membership in Firestore so `/api/me` succeeds. In the Firebase console, create `users/{your-uid}` with fields `clubId: "club-1"`, `role: "admin"`, `teamIds: ["team-1"]`. Your UID is shown in Authentication → Users after your first sign-in attempt.

Run the Go server (`make build && ./bin/server`) and the web dev server (`cd web && npm run dev`), open `http://localhost:5173`, and sign in.
Expected: the shell renders with the Squad and Matches navigation.

- [ ] **Step 11: Run checks**

Run: `cd web && npx biome ci . && npx tsc --noEmit && npm test -- --run`
Expected: all pass.

- [ ] **Step 12: Stage**

```bash
git add web .gitignore
```
Commit message: `feat(web): Firebase sign-in, API client and app shell`

---

## Task 12: Squad management UI

**Files:**
- Create: `web/src/features/players/api.ts`, `web/src/features/players/SquadPage.tsx`, `web/src/features/players/PlayerForm.tsx`, `web/src/features/players/positions.ts`
- Modify: `web/src/main.tsx`
- Test: `web/src/features/players/PlayerForm.test.tsx`

**Interfaces:**
- Consumes: `useAuth` (Task 11), `core.Player`, `core.PositionGroup` from `web/src/types/core` (Task 10), player endpoints (Task 10)
- Produces:
  - `usePlayers()`, `useSavePlayer()`, `useDeletePlayer()` hooks
  - `POSITION_GROUP_LABELS: Record<PositionGroup, string>`
  - `<PlayerForm player? onSubmit onCancel>`

- [ ] **Step 1: Write the position labels**

Create `web/src/features/players/positions.ts`:

```ts
import type { PositionGroup } from "../../types/core";

export const POSITION_GROUP_LABELS: Record<PositionGroup, string> = {
  front_row: "Front row",
  second_row: "Second row",
  back_row: "Back row",
  half_backs: "Half backs",
  centres: "Centres",
  back_three: "Back three",
};

export const POSITION_GROUPS = Object.keys(POSITION_GROUP_LABELS) as PositionGroup[];
```

If `tsc` reports that `Record<PositionGroup, string>` is missing a key, the Go constants and this map have diverged — add the missing entry rather than loosening the type. That exhaustiveness check is the point.

- [ ] **Step 2: Write the failing form test**

Create `web/src/features/players/PlayerForm.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PlayerForm } from "./PlayerForm";

describe("PlayerForm", () => {
  it("submits the entered player", async () => {
    const onSubmit = vi.fn();
    render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

    await userEvent.type(screen.getByLabelText(/first name/i), "Thomas");
    await userEvent.type(screen.getByLabelText(/last name/i), "Lefevre");
    await userEvent.click(screen.getByLabelText(/back row/i));
    await userEvent.click(screen.getByRole("button", { name: /save/i }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        firstName: "Thomas",
        lastName: "Lefevre",
        positions: ["back_row"],
        status: "active",
      }),
    );
  });

  it("refuses to submit without a position", async () => {
    const onSubmit = vi.fn();
    render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

    await userEvent.type(screen.getByLabelText(/first name/i), "Thomas");
    await userEvent.type(screen.getByLabelText(/last name/i), "Lefevre");
    await userEvent.click(screen.getByRole("button", { name: /save/i }));

    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(/position/i);
  });

  it("pre-fills when editing", () => {
    render(
      <PlayerForm
        player={{
          id: "p1",
          clubId: "club-1",
          firstName: "Marie",
          lastName: "Girard",
          dob: "2001-02-03",
          positions: ["second_row"],
          teamIds: ["team-1"],
          status: "active",
        }}
        onSubmit={() => {}}
        onCancel={() => {}}
      />,
    );

    expect(screen.getByLabelText(/first name/i)).toHaveValue("Marie");
    expect(screen.getByLabelText(/second row/i)).toBeChecked();
  });
});
```

Install the extra test dependencies and register the DOM matchers:

```bash
cd web && npm install -D @testing-library/user-event
```

Create `web/src/setupTests.ts` with `import "@testing-library/jest-dom";` and add `setupFiles: ["./src/setupTests.ts"]` to the `test` block in `web/vite.config.ts`.

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd web && npm test -- --run src/features/players/PlayerForm.test.tsx`
Expected: FAIL — cannot resolve `./PlayerForm`.

- [ ] **Step 4: Implement the form**

Create `web/src/features/players/PlayerForm.tsx`:

```tsx
import { type FormEvent, useState } from "react";
import type { Player, PositionGroup } from "../../types/core";
import { POSITION_GROUPS, POSITION_GROUP_LABELS } from "./positions";

type Props = {
  player?: Player;
  onSubmit: (player: Player) => void;
  onCancel: () => void;
};

export function PlayerForm({ player, onSubmit, onCancel }: Props) {
  const [firstName, setFirstName] = useState(player?.firstName ?? "");
  const [lastName, setLastName] = useState(player?.lastName ?? "");
  const [dob, setDob] = useState(player?.dob ?? "");
  const [positions, setPositions] = useState<PositionGroup[]>(player?.positions ?? []);
  const [error, setError] = useState<string | null>(null);

  function togglePosition(group: PositionGroup) {
    setPositions((current) =>
      current.includes(group) ? current.filter((g) => g !== group) : [...current, group],
    );
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();

    if (!firstName.trim() || !lastName.trim()) {
      setError("First and last name are required.");
      return;
    }
    if (positions.length === 0) {
      setError("Select at least one position group.");
      return;
    }

    setError(null);
    onSubmit({
      id: player?.id ?? "",
      clubId: player?.clubId ?? "",
      firstName: firstName.trim(),
      lastName: lastName.trim(),
      dob,
      positions,
      teamIds: player?.teamIds ?? [],
      status: player?.status ?? "active",
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 rounded-xl bg-white p-5 shadow-sm">
      {error && (
        <p role="alert" className="rounded bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block text-sm">
          <span className="mb-1 block font-medium">First name</span>
          <input
            value={firstName}
            onChange={(e) => setFirstName(e.target.value)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Last name</span>
          <input
            value={lastName}
            onChange={(e) => setLastName(e.target.value)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
      </div>

      <label className="block text-sm">
        <span className="mb-1 block font-medium">Date of birth (optional)</span>
        <input
          type="date"
          value={dob}
          onChange={(e) => setDob(e.target.value)}
          className="rounded border border-slate-300 px-3 py-2"
        />
      </label>

      <fieldset>
        <legend className="mb-2 text-sm font-medium">Position groups</legend>
        <div className="flex flex-wrap gap-3">
          {POSITION_GROUPS.map((group) => (
            <label key={group} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={positions.includes(group)}
                onChange={() => togglePosition(group)}
              />
              {POSITION_GROUP_LABELS[group]}
            </label>
          ))}
        </div>
      </fieldset>

      <div className="flex gap-3">
        <button type="submit" className="rounded bg-slate-900 px-4 py-2 font-semibold text-white">
          Save
        </button>
        <button type="button" onClick={onCancel} className="rounded px-4 py-2 text-slate-600">
          Cancel
        </button>
      </div>
    </form>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd web && npm test -- --run src/features/players/PlayerForm.test.tsx`
Expected: PASS — three tests.

- [ ] **Step 6: Write the data hooks**

Create `web/src/features/players/api.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../../lib/auth";
import type { Player } from "../../types/core";

const PLAYERS_KEY = ["players"];

export function usePlayers() {
  const { apiFetch } = useAuth();
  return useQuery({
    queryKey: PLAYERS_KEY,
    queryFn: () => apiFetch<Player[]>("/players"),
  });
}

export function useSavePlayer() {
  const { apiFetch } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (player: Player) =>
      player.id
        ? apiFetch<Player>(`/players/${player.id}`, {
            method: "PUT",
            body: JSON.stringify(player),
          })
        : apiFetch<Player>("/players", { method: "POST", body: JSON.stringify(player) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PLAYERS_KEY }),
  });
}

export function useDeletePlayer() {
  const { apiFetch } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (playerID: string) => apiFetch<void>(`/players/${playerID}`, { method: "DELETE" }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PLAYERS_KEY }),
  });
}
```

- [ ] **Step 7: Write the squad page**

Create `web/src/features/players/SquadPage.tsx`:

```tsx
import { useState } from "react";
import { useAuth } from "../../lib/auth";
import type { Player } from "../../types/core";
import { PlayerForm } from "./PlayerForm";
import { useDeletePlayer, usePlayers, useSavePlayer } from "./api";
import { POSITION_GROUP_LABELS } from "./positions";

export function SquadPage() {
  const { membership } = useAuth();
  const { data: players, isPending, error } = usePlayers();
  const savePlayer = useSavePlayer();
  const deletePlayer = useDeletePlayer();
  const [editing, setEditing] = useState<Player | null>(null);
  const [adding, setAdding] = useState(false);

  const canManage = membership?.role === "admin";

  if (isPending) return <p className="text-slate-500">Loading squad…</p>;
  if (error) return <p className="text-red-700">Could not load the squad.</p>;

  function handleSubmit(player: Player) {
    savePlayer.mutate(player, {
      onSuccess: () => {
        setEditing(null);
        setAdding(false);
      },
    });
  }

  return (
    <section className="space-y-5">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Squad</h1>
        {canManage && !adding && !editing && (
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="rounded bg-slate-900 px-4 py-2 text-sm font-semibold text-white"
          >
            Add player
          </button>
        )}
      </div>

      {(adding || editing) && (
        <PlayerForm
          player={editing ?? undefined}
          onSubmit={handleSubmit}
          onCancel={() => {
            setEditing(null);
            setAdding(false);
          }}
        />
      )}

      <ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
        {players?.map((player) => (
          <li key={player.id} className="flex items-center gap-4 px-5 py-3">
            <div className="flex-1">
              <p className="font-medium">
                {player.firstName} {player.lastName}
              </p>
              <p className="text-sm text-slate-500">
                {player.positions.map((g) => POSITION_GROUP_LABELS[g]).join(" · ")}
              </p>
            </div>
            {canManage && (
              <>
                <button
                  type="button"
                  onClick={() => setEditing(player)}
                  className="text-sm text-slate-600"
                >
                  Edit
                </button>
                <button
                  type="button"
                  onClick={() => deletePlayer.mutate(player.id)}
                  className="text-sm text-red-700"
                >
                  Remove
                </button>
              </>
            )}
          </li>
        ))}
        {players?.length === 0 && <li className="px-5 py-6 text-slate-500">No players yet.</li>}
      </ul>
    </section>
  );
}
```

- [ ] **Step 8: Add the route**

In `web/src/main.tsx`, import `SquadPage` and add to the shell's `children`:

```tsx
      { path: "squad", element: <SquadPage /> },
```

- [ ] **Step 9: Verify in the browser**

With the Go server and `npm run dev` both running, open `http://localhost:5173/squad`, add a player, edit them, and remove them.
Expected: each action persists — reload the page and the list matches.

- [ ] **Step 10: Run checks**

Run: `cd web && npx biome ci . && npx tsc --noEmit && npm test -- --run`
Expected: all pass.

- [ ] **Step 11: Stage**

```bash
git add web
```
Commit message: `feat(web): squad list and player form`

---

## Task 13: Core match and lineup domain

**Files:**
- Create: `internal/core/match.go`
- Test: `internal/core/match_test.go`

**Interfaces:**
- Consumes: `core.ValidationError` (Task 7)
- Produces:
  - `core.Venue` constants: `VenueHome`, `VenueAway`, `VenueNeutral`
  - `core.MatchStatus` constants: `MatchScheduled`, `MatchInProgress`, `MatchCompleted`
  - `core.LineupSlot{Jersey int; PlayerID string}`
  - `core.Lineup{Starters, Bench []LineupSlot}` with `IsEmpty() bool` and `Validate(eligible map[string]bool) error`
  - `core.Match{...}` with `Validate() error`

- [ ] **Step 1: Write the failing test**

Create `internal/core/match_test.go`:

```go
package core

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func fullStarters() []LineupSlot {
	slots := make([]LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		slots = append(slots, LineupSlot{Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey)})
	}
	return slots
}

func eligibleSquad() map[string]bool {
	squad := map[string]bool{}
	for jersey := 1; jersey <= 23; jersey++ {
		squad["player-"+strconv.Itoa(jersey)] = true
	}
	return squad
}

func validMatch() Match {
	return Match{
		ID:          "match-1",
		ClubID:      "club-1",
		TeamID:      "team-1",
		SeasonID:    "2026-27",
		Opponent:    "Castelnau RC",
		KickoffAt:   time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Competition: "Regional 1",
		Venue:       VenueHome,
		Status:      MatchScheduled,
	}
}

func TestMatchValidateAcceptsAValidMatch(t *testing.T) {
	if err := validMatch().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestMatchValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Match)
		field  string
	}{
		{"no opponent", func(m *Match) { m.Opponent = " " }, "opponent"},
		{"no kickoff", func(m *Match) { m.KickoffAt = time.Time{} }, "kickoffAt"},
		{"unknown venue", func(m *Match) { m.Venue = "the moon" }, "venue"},
		{"unknown status", func(m *Match) { m.Status = "abandoned-ish" }, "status"},
		{"no season", func(m *Match) { m.SeasonID = "" }, "seasonId"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := validMatch()
			c.mutate(&m)

			err := m.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, c.field)
			}
		})
	}
}

func TestLineupIsEmpty(t *testing.T) {
	if !(Lineup{}).IsEmpty() {
		t.Error("zero Lineup should be empty")
	}
	if (Lineup{Starters: fullStarters()}).IsEmpty() {
		t.Error("Lineup with starters should not be empty")
	}
}

func TestLineupValidateAcceptsAFullXV(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 16, PlayerID: "player-16"}, {Jersey: 17, PlayerID: "player-17"}},
	}

	if err := lineup.Validate(eligibleSquad()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestLineupValidateRequiresAllFifteenJerseys(t *testing.T) {
	lineup := Lineup{Starters: fullStarters()[:14]}

	err := lineup.Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a 14-man starting lineup")
	}
	if !strings.Contains(err.Error(), "starters") {
		t.Errorf("error = %q, want it to name the starters field", err)
	}
}

func TestLineupValidateRejectsDuplicateJersey(t *testing.T) {
	starters := fullStarters()
	starters[14].Jersey = 1 // two players wearing 1, and nobody wearing 15

	if err := Lineup{Starters: starters}.Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for a duplicated jersey")
	}
}

func TestLineupValidateRejectsSamePlayerTwice(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 16, PlayerID: "player-7"}},
	}

	err := lineup.Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a player named twice")
	}
	if !strings.Contains(err.Error(), "player-7") {
		t.Errorf("error = %q, want it to name the duplicated player", err)
	}
}

func TestLineupValidateRejectsBenchJerseyOutOfRange(t *testing.T) {
	lineup := Lineup{
		Starters: fullStarters(),
		Bench:    []LineupSlot{{Jersey: 24, PlayerID: "player-16"}},
	}

	if err := lineup.Validate(eligibleSquad()); err == nil {
		t.Error("Validate() error = nil, want an error for bench jersey 24")
	}
}

func TestLineupValidateRejectsIneligiblePlayer(t *testing.T) {
	starters := fullStarters()
	starters[0].PlayerID = "player-from-another-club"

	err := Lineup{Starters: starters}.Validate(eligibleSquad())
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a player outside the squad")
	}
	if !strings.Contains(err.Error(), "player-from-another-club") {
		t.Errorf("error = %q, want it to name the ineligible player", err)
	}
}
```

Selecting the same player twice and selecting someone from another club are the two mistakes that silently corrupt every downstream minutes calculation, so both get their own test.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/ -run 'TestMatch|TestLineup'`
Expected: FAIL — `undefined: Match`, `undefined: Lineup`.

- [ ] **Step 3: Implement the match domain**

Create `internal/core/match.go`:

```go
package core

import (
	"strconv"
	"strings"
	"time"
)

type Venue string

const (
	VenueHome    Venue = "home"
	VenueAway    Venue = "away"
	VenueNeutral Venue = "neutral"
)

type MatchStatus string

const (
	MatchScheduled  MatchStatus = "scheduled"
	MatchInProgress MatchStatus = "in_progress"
	MatchCompleted  MatchStatus = "completed"
)

// LineupSlot ties a jersey number to the player wearing it.
type LineupSlot struct {
	Jersey   int    `json:"jersey" firestore:"jersey"`
	PlayerID string `json:"playerId" firestore:"playerId"`
}

// Lineup is the selected squad for one match: jerseys 1-15 start, 16-23 are
// replacements.
type Lineup struct {
	Starters []LineupSlot `json:"starters" firestore:"starters"`
	Bench    []LineupSlot `json:"bench" firestore:"bench"`
}

func (l Lineup) IsEmpty() bool {
	return len(l.Starters) == 0 && len(l.Bench) == 0
}

// Validate checks that the lineup is a legal selection from the eligible squad.
// A player may hold exactly one jersey, and jerseys 1-15 must all be filled.
func (l Lineup) Validate(eligible map[string]bool) error {
	seenJersey := map[int]bool{}
	seenPlayer := map[string]bool{}

	check := func(slots []LineupSlot, field string, low, high int) error {
		for _, slot := range slots {
			if slot.Jersey < low || slot.Jersey > high {
				return ValidationError{
					Field:  field,
					Reason: "jersey " + strconv.Itoa(slot.Jersey) + " is outside " + strconv.Itoa(low) + "-" + strconv.Itoa(high),
				}
			}
			if seenJersey[slot.Jersey] {
				return ValidationError{Field: field, Reason: "jersey " + strconv.Itoa(slot.Jersey) + " is assigned twice"}
			}
			seenJersey[slot.Jersey] = true

			if slot.PlayerID == "" {
				return ValidationError{Field: field, Reason: "jersey " + strconv.Itoa(slot.Jersey) + " has no player"}
			}
			if seenPlayer[slot.PlayerID] {
				return ValidationError{Field: field, Reason: "player " + slot.PlayerID + " is selected twice"}
			}
			seenPlayer[slot.PlayerID] = true

			if !eligible[slot.PlayerID] {
				return ValidationError{Field: field, Reason: "player " + slot.PlayerID + " is not in this team's squad"}
			}
		}
		return nil
	}

	if err := check(l.Starters, "starters", 1, 15); err != nil {
		return err
	}
	if err := check(l.Bench, "bench", 16, 23); err != nil {
		return err
	}

	for jersey := 1; jersey <= 15; jersey++ {
		if !seenJersey[jersey] {
			return ValidationError{Field: "starters", Reason: "jersey " + strconv.Itoa(jersey) + " is unfilled"}
		}
	}
	return nil
}

type Match struct {
	ID          string      `json:"id,omitempty" firestore:"-"`
	ClubID      string      `json:"clubId,omitempty" firestore:"-"`
	TeamID      string      `json:"teamId,omitempty" firestore:"-"`
	SeasonID    string      `json:"seasonId" firestore:"seasonId"`
	Opponent    string      `json:"opponent" firestore:"opponent"`
	KickoffAt   time.Time   `json:"kickoffAt" firestore:"kickoffAt"`
	Competition string      `json:"competition" firestore:"competition"`
	Venue       Venue       `json:"venue" firestore:"venue"`
	Status      MatchStatus `json:"status" firestore:"status"`
	Lineup      Lineup      `json:"lineup" firestore:"lineup"`
}

func (m Match) Validate() error {
	if strings.TrimSpace(m.Opponent) == "" {
		return ValidationError{Field: "opponent", Reason: "must not be empty"}
	}
	if m.KickoffAt.IsZero() {
		return ValidationError{Field: "kickoffAt", Reason: "must be set"}
	}
	if strings.TrimSpace(m.SeasonID) == "" {
		return ValidationError{Field: "seasonId", Reason: "must not be empty"}
	}
	switch m.Venue {
	case VenueHome, VenueAway, VenueNeutral:
	default:
		return ValidationError{Field: "venue", Reason: "must be home, away or neutral"}
	}
	switch m.Status {
	case MatchScheduled, MatchInProgress, MatchCompleted:
	default:
		return ValidationError{Field: "status", Reason: "must be scheduled, in_progress or completed"}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/ -v`
Expected: PASS — all core tests including the nine new match and lineup tests.

- [ ] **Step 5: Stage**

```bash
git add internal/core
```
Commit message: `feat(core): match and lineup domain with selection rules`

---

## Task 14: Match service, store and endpoints

**Files:**
- Create: `internal/fixture/service.go`, `internal/fixture/service_test.go`, `internal/store/firestore/match.go`, `internal/store/firestore/match_test.go`, `internal/httpapi/matches.go`, `internal/httpapi/matches_test.go`
- Modify: `internal/httpapi/router.go`, `cmd/server/main.go`
- Test: as above

**Interfaces:**
- Consumes: `core.Match`, `core.Lineup` (Task 13), `core.Player` (Task 7), `(*Store).clubDoc` (Task 4), `Deps.protected` (Task 10)
- Produces:
  - `fixture.MatchReader interface { Match(ctx, clubID, teamID, matchID string) (core.Match, error); Matches(ctx, clubID, teamID string) ([]core.Match, error) }`
  - `fixture.MatchWriter interface { PutMatch(ctx, clubID, teamID string, m core.Match) error }`
  - `fixture.SquadReader interface { Players(ctx, clubID string) ([]core.Player, error) }`
  - `fixture.NewService(r MatchReader, w MatchWriter, s SquadReader, newID func() string) *Service`
  - `(*Service).Schedule`, `(*Service).Get`, `(*Service).List`, `(*Service).SetLineup`
  - `(*Store).Match`, `(*Store).Matches`, `(*Store).PutMatch`
  - Routes: `GET|POST /api/teams/{teamID}/matches`, `GET /api/teams/{teamID}/matches/{matchID}`, `PUT /api/teams/{teamID}/matches/{matchID}/lineup`

- [ ] **Step 1: Write the failing service test**

Create `internal/fixture/service_test.go`:

```go
package fixture

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

type memStore struct {
	matches map[string]core.Match // key: clubID/teamID/matchID
	squad   []core.Player
}

func newMemStore() *memStore {
	return &memStore{matches: map[string]core.Match{}}
}

func key(clubID, teamID, matchID string) string { return clubID + "/" + teamID + "/" + matchID }

func (m *memStore) Match(_ context.Context, clubID, teamID, matchID string) (core.Match, error) {
	match, ok := m.matches[key(clubID, teamID, matchID)]
	if !ok {
		return core.Match{}, core.ErrNotFound
	}
	return match, nil
}

func (m *memStore) Matches(_ context.Context, clubID, teamID string) ([]core.Match, error) {
	var out []core.Match
	for k, match := range m.matches {
		if k[:len(clubID)+len(teamID)+2] == clubID+"/"+teamID+"/" {
			out = append(out, match)
		}
	}
	return out, nil
}

func (m *memStore) PutMatch(_ context.Context, clubID, teamID string, match core.Match) error {
	m.matches[key(clubID, teamID, match.ID)] = match
	return nil
}

func (m *memStore) Players(_ context.Context, _ string) ([]core.Player, error) {
	return m.squad, nil
}

// squadOf builds n players, all belonging to team-1.
func squadOf(n int) []core.Player {
	players := make([]core.Player, 0, n)
	for i := 1; i <= n; i++ {
		players = append(players, core.Player{
			ID:        "player-" + strconv.Itoa(i),
			FirstName: "First",
			LastName:  "Last" + strconv.Itoa(i),
			Positions: []core.PositionGroup{core.BackRow},
			TeamIDs:   []string{"team-1"},
			Status:    core.PlayerActive,
		})
	}
	return players
}

func fullLineup() core.Lineup {
	starters := make([]core.LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		starters = append(starters, core.LineupSlot{Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey)})
	}
	return core.Lineup{Starters: starters}
}

func newMatch() core.Match {
	return core.Match{
		SeasonID:    "2026-27",
		Opponent:    "Castelnau RC",
		KickoffAt:   time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Competition: "Regional 1",
		Venue:       core.VenueHome,
	}
}

func newTestService(store *memStore) *Service {
	return NewService(store, store, store, func() string { return "match-generated" })
}

func TestScheduleDefaultsStatusAndAssignsIdentity(t *testing.T) {
	store := newMemStore()
	svc := newTestService(store)

	got, err := svc.Schedule(context.Background(), "club-1", "team-1", newMatch())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if got.ID != "match-generated" || got.ClubID != "club-1" || got.TeamID != "team-1" {
		t.Errorf("identity = (%q, %q, %q), want (match-generated, club-1, team-1)", got.ID, got.ClubID, got.TeamID)
	}
	if got.Status != core.MatchScheduled {
		t.Errorf("Status = %q, want %q", got.Status, core.MatchScheduled)
	}
}

func TestScheduleRejectsInvalidMatch(t *testing.T) {
	svc := newTestService(newMemStore())

	m := newMatch()
	m.Opponent = ""

	if _, err := svc.Schedule(context.Background(), "club-1", "team-1", m); err == nil {
		t.Error("Schedule() error = nil, want a validation error")
	}
}

func TestSetLineupAcceptsAValidSelection(t *testing.T) {
	store := newMemStore()
	store.squad = squadOf(15)
	svc := newTestService(store)

	scheduled, err := svc.Schedule(context.Background(), "club-1", "team-1", newMatch())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	got, err := svc.SetLineup(context.Background(), "club-1", "team-1", scheduled.ID, fullLineup())
	if err != nil {
		t.Fatalf("SetLineup() error = %v", err)
	}
	if len(got.Lineup.Starters) != 15 {
		t.Errorf("len(Starters) = %d, want 15", len(got.Lineup.Starters))
	}
}

func TestSetLineupRejectsPlayerFromAnotherTeam(t *testing.T) {
	store := newMemStore()
	store.squad = squadOf(15)
	store.squad[0].TeamIDs = []string{"team-2"} // player-1 is a colt, not in team-1
	svc := newTestService(store)

	scheduled, _ := svc.Schedule(context.Background(), "club-1", "team-1", newMatch())

	_, err := svc.SetLineup(context.Background(), "club-1", "team-1", scheduled.ID, fullLineup())
	if err == nil {
		t.Fatal("SetLineup() error = nil, want an eligibility error")
	}
	var invalid core.ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("error = %v, want a core.ValidationError", err)
	}
}

func TestSetLineupOnUnknownMatchIsNotFound(t *testing.T) {
	store := newMemStore()
	store.squad = squadOf(15)
	svc := newTestService(store)

	_, err := svc.SetLineup(context.Background(), "club-1", "team-1", "no-such-match", fullLineup())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fixture/`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Implement the service**

Create `internal/fixture/service.go`:

```go
// Package fixture holds the match-scheduling use-cases. "Fixture" is the rugby
// sense of the word: a scheduled match.
package fixture

import (
	"context"
	"fmt"
	"slices"

	"pitch-ai/internal/core"
)

type MatchReader interface {
	Match(ctx context.Context, clubID, teamID, matchID string) (core.Match, error)
	Matches(ctx context.Context, clubID, teamID string) ([]core.Match, error)
}

type MatchWriter interface {
	PutMatch(ctx context.Context, clubID, teamID string, m core.Match) error
}

// SquadReader supplies the players a lineup may be selected from.
type SquadReader interface {
	Players(ctx context.Context, clubID string) ([]core.Player, error)
}

type Service struct {
	reader MatchReader
	writer MatchWriter
	squad  SquadReader
	newID  func() string
}

func NewService(r MatchReader, w MatchWriter, s SquadReader, newID func() string) *Service {
	return &Service{reader: r, writer: w, squad: s, newID: newID}
}

// Schedule creates a match. Identity always comes from the caller's context.
func (s *Service) Schedule(ctx context.Context, clubID, teamID string, m core.Match) (core.Match, error) {
	m.ID = s.newID()
	m.ClubID = clubID
	m.TeamID = teamID
	if m.Status == "" {
		m.Status = core.MatchScheduled
	}

	if err := m.Validate(); err != nil {
		return core.Match{}, err
	}
	if err := s.writer.PutMatch(ctx, clubID, teamID, m); err != nil {
		return core.Match{}, fmt.Errorf("fixture: schedule match: %w", err)
	}
	return m, nil
}

func (s *Service) Get(ctx context.Context, clubID, teamID, matchID string) (core.Match, error) {
	return s.reader.Match(ctx, clubID, teamID, matchID)
}

func (s *Service) List(ctx context.Context, clubID, teamID string) ([]core.Match, error) {
	return s.reader.Matches(ctx, clubID, teamID)
}

// SetLineup replaces a match's selection, checking every player is an active
// member of this team's squad.
func (s *Service) SetLineup(ctx context.Context, clubID, teamID, matchID string, lineup core.Lineup) (core.Match, error) {
	match, err := s.reader.Match(ctx, clubID, teamID, matchID)
	if err != nil {
		return core.Match{}, err
	}

	eligible, err := s.eligiblePlayers(ctx, clubID, teamID)
	if err != nil {
		return core.Match{}, err
	}
	if err := lineup.Validate(eligible); err != nil {
		return core.Match{}, err
	}

	match.Lineup = lineup
	if err := s.writer.PutMatch(ctx, clubID, teamID, match); err != nil {
		return core.Match{}, fmt.Errorf("fixture: set lineup on %q: %w", matchID, err)
	}
	return match, nil
}

func (s *Service) eligiblePlayers(ctx context.Context, clubID, teamID string) (map[string]bool, error) {
	players, err := s.squad.Players(ctx, clubID)
	if err != nil {
		return nil, fmt.Errorf("fixture: load squad: %w", err)
	}

	eligible := make(map[string]bool, len(players))
	for _, p := range players {
		if p.Status == core.PlayerActive && slices.Contains(p.TeamIDs, teamID) {
			eligible[p.ID] = true
		}
	}
	return eligible, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fixture/ -v`
Expected: PASS — five tests.

- [ ] **Step 5: Write the failing store test**

Create `internal/store/firestore/match_test.go`:

```go
package firestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

func seedMatch(t *testing.T, store *Store, clubID, teamID, id, opponent string) core.Match {
	t.Helper()
	m := core.Match{
		ID:          id,
		ClubID:      clubID,
		TeamID:      teamID,
		SeasonID:    "2026-27",
		Opponent:    opponent,
		KickoffAt:   time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Competition: "Regional 1",
		Venue:       core.VenueHome,
		Status:      core.MatchScheduled,
	}
	if err := store.PutMatch(context.Background(), clubID, teamID, m); err != nil {
		t.Fatalf("PutMatch() error = %v", err)
	}
	return m
}

func TestMatchRoundTrip(t *testing.T) {
	store := newTestStore(t)
	want := seedMatch(t, store, "club-match-rt", "team-1", "match-1", "Castelnau RC")

	got, err := store.Match(context.Background(), "club-match-rt", "team-1", "match-1")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.Opponent != want.Opponent || got.ID != "match-1" || got.TeamID != "team-1" {
		t.Errorf("Match() = %+v, want %+v", got, want)
	}
	if !got.KickoffAt.Equal(want.KickoffAt) {
		t.Errorf("KickoffAt = %v, want %v", got.KickoffAt, want.KickoffAt)
	}
}

func TestMatchMissingIsErrNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Match(context.Background(), "club-none", "team-1", "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Match() error = %v, want core.ErrNotFound", err)
	}
}

func TestMatchesAreScopedToTheirTeam(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-match-scope"

	seedMatch(t, store, clubID, "team-1", "m1", "Castelnau RC")
	seedMatch(t, store, clubID, "team-1", "m2", "Lavaur")
	seedMatch(t, store, clubID, "team-2", "m3", "Colts opposition")

	list, err := store.Matches(context.Background(), clubID, "team-1")
	if err != nil {
		t.Fatalf("Matches() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(Matches()) = %d, want 2", len(list))
	}
}

func TestPutMatchPersistsLineup(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-match-lineup"
	m := seedMatch(t, store, clubID, "team-1", "m-lineup", "Castelnau RC")

	m.Lineup = core.Lineup{
		Starters: []core.LineupSlot{{Jersey: 7, PlayerID: "player-7"}},
		Bench:    []core.LineupSlot{{Jersey: 16, PlayerID: "player-16"}},
	}
	if err := store.PutMatch(context.Background(), clubID, "team-1", m); err != nil {
		t.Fatalf("PutMatch() error = %v", err)
	}

	got, err := store.Match(context.Background(), clubID, "team-1", "m-lineup")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(got.Lineup.Starters) != 1 || got.Lineup.Starters[0].Jersey != 7 {
		t.Errorf("Lineup.Starters = %+v, want jersey 7", got.Lineup.Starters)
	}
	if len(got.Lineup.Bench) != 1 || got.Lineup.Bench[0].PlayerID != "player-16" {
		t.Errorf("Lineup.Bench = %+v, want player-16", got.Lineup.Bench)
	}
}
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `make test-store` (with `make emulator` running)
Expected: FAIL — `store.PutMatch undefined`.

- [ ] **Step 7: Implement the match store**

Create `internal/store/firestore/match.go`:

```go
package firestore

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pitch-ai/internal/core"
)

func (s *Store) matchesCol(clubID, teamID string) *fs.CollectionRef {
	return s.clubDoc(clubID).Collection("teams").Doc(teamID).Collection("matches")
}

func (s *Store) Match(ctx context.Context, clubID, teamID, matchID string) (core.Match, error) {
	snap, err := s.matchesCol(clubID, teamID).Doc(matchID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return core.Match{}, core.ErrNotFound
	}
	if err != nil {
		return core.Match{}, fmt.Errorf("firestore: get match %q: %w", matchID, err)
	}
	return matchFromSnap(snap, clubID, teamID)
}

func (s *Store) Matches(ctx context.Context, clubID, teamID string) ([]core.Match, error) {
	iter := s.matchesCol(clubID, teamID).OrderBy("kickoffAt", fs.Desc).Documents(ctx)
	defer iter.Stop()

	var out []core.Match
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore: list matches: %w", err)
		}
		m, err := matchFromSnap(snap, clubID, teamID)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
}

func (s *Store) PutMatch(ctx context.Context, clubID, teamID string, m core.Match) error {
	if _, err := s.matchesCol(clubID, teamID).Doc(m.ID).Set(ctx, m); err != nil {
		return fmt.Errorf("firestore: put match %q: %w", m.ID, err)
	}
	return nil
}

func matchFromSnap(snap *fs.DocumentSnapshot, clubID, teamID string) (core.Match, error) {
	var m core.Match
	if err := snap.DataTo(&m); err != nil {
		return core.Match{}, fmt.Errorf("firestore: decode match %q: %w", snap.Ref.ID, err)
	}
	m.ID = snap.Ref.ID
	m.ClubID = clubID
	m.TeamID = teamID
	return m, nil
}
```

- [ ] **Step 8: Run the store tests**

Run: `make test-store`
Expected: PASS — membership, player and match tests.

- [ ] **Step 9: Write the failing handler test**

Create `internal/httpapi/matches_test.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
	"pitch-ai/internal/fixture"
)

type fakeMatchStore struct {
	matches map[string]core.Match
	squad   []core.Player
}

func newFakeMatchStore() *fakeMatchStore {
	return &fakeMatchStore{matches: map[string]core.Match{}}
}

func (f *fakeMatchStore) Match(_ context.Context, clubID, teamID, matchID string) (core.Match, error) {
	m, ok := f.matches[clubID+"/"+teamID+"/"+matchID]
	if !ok {
		return core.Match{}, core.ErrNotFound
	}
	return m, nil
}

func (f *fakeMatchStore) Matches(_ context.Context, clubID, teamID string) ([]core.Match, error) {
	var out []core.Match
	for _, m := range f.matches {
		if m.ClubID == clubID && m.TeamID == teamID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMatchStore) PutMatch(_ context.Context, clubID, teamID string, m core.Match) error {
	f.matches[clubID+"/"+teamID+"/"+m.ID] = m
	return nil
}

func (f *fakeMatchStore) Players(_ context.Context, _ string) ([]core.Player, error) {
	return f.squad, nil
}

const validMatchJSON = `{"seasonId":"2026-27","opponent":"Castelnau RC","kickoffAt":"2026-03-14T15:00:00Z","competition":"Regional 1","venue":"home","status":"scheduled","lineup":{"starters":null,"bench":null}}`

func matchRouter(t *testing.T, role auth.Role) (http.Handler, *fakeMatchStore) {
	t.Helper()
	store := newFakeMatchStore()
	svc := fixture.NewService(store, store, store, func() string { return "match-generated" })

	return NewRouter(Deps{
		Logger:  discardLogger(),
		Fixture: svc,
		Auth: fakeAuthMiddleware(auth.Membership{
			UID: "uid-1", ClubID: "club-1", TeamIDs: []string{"team-1"}, Role: role,
		}),
	}), store
}

func TestCreateMatch(t *testing.T) {
	router, store := matchRouter(t, auth.RoleAdmin)

	rec := do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var got core.Match
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.TeamID != "team-1" || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (club-1, team-1)", got.ClubID, got.TeamID)
	}
	if _, ok := store.matches["club-1/team-1/match-generated"]; !ok {
		t.Error("match was not persisted under the caller's club and team")
	}
}

func TestCreateMatchRejectsTeamOutsideMembership(t *testing.T) {
	router, _ := matchRouter(t, auth.RoleAdmin)

	rec := do(t, router, http.MethodPost, "/api/teams/team-99/matches", validMatchJSON)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a team the caller does not belong to", rec.Code)
	}
}

func TestSetLineupRejectsIncompleteXV(t *testing.T) {
	router, store := matchRouter(t, auth.RoleAdmin)
	store.squad = []core.Player{{
		ID: "player-1", FirstName: "A", LastName: "B",
		Positions: []core.PositionGroup{core.BackRow}, TeamIDs: []string{"team-1"}, Status: core.PlayerActive,
	}}
	_ = do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)

	body := `{"starters":[{"jersey":1,"playerId":"player-1"}],"bench":null}`
	rec := do(t, router, http.MethodPut, "/api/teams/team-1/matches/match-generated/lineup", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a one-man lineup", rec.Code)
	}
}

func TestCoachCanManageMatches(t *testing.T) {
	router, _ := matchRouter(t, auth.RoleCoach)

	if rec := do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON); rec.Code != http.StatusCreated {
		t.Errorf("POST status = %d, want 201 — a coach may create fixtures", rec.Code)
	}
	if rec := do(t, router, http.MethodGet, "/api/teams/team-1/matches", ""); rec.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rec.Code)
	}
}
```

`discardLogger`, `fakeAuthMiddleware` and `do` already exist in `internal/httpapi/testing_test.go` from Task 10 — this test reuses them unchanged.

- [ ] **Step 10: Run the test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestCreateMatch`
Expected: FAIL — `Deps.Fixture undefined`.

- [ ] **Step 11: Implement the match handlers**

Create `internal/httpapi/matches.go`:

```go
package httpapi

import (
	"net/http"
	"slices"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

// teamFromPath returns the requested team, rejecting any team the caller is not
// a member of. Tenancy is enforced here, never trusted from the path alone.
func teamFromPath(w http.ResponseWriter, r *http.Request) (auth.Membership, string, bool) {
	member, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return auth.Membership{}, "", false
	}

	teamID := r.PathValue("teamID")
	if !slices.Contains(member.TeamIDs, teamID) {
		writeError(w, http.StatusForbidden, "not a member of this team")
		return auth.Membership{}, "", false
	}
	return member, teamID, true
}

func (d Deps) handleListMatches(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	matches, err := d.Fixture.List(r.Context(), member.ClubID, teamID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	if matches == nil {
		matches = []core.Match{}
	}
	writeJSON(w, http.StatusOK, matches)
}

func (d Deps) handleGetMatch(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	match, err := d.Fixture.Get(r.Context(), member.ClubID, teamID, r.PathValue("matchID"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, match)
}

func (d Deps) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body core.Match
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	created, err := d.Fixture.Schedule(r.Context(), member.ClubID, teamID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (d Deps) handleSetLineup(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body core.Lineup
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	updated, err := d.Fixture.SetLineup(r.Context(), member.ClubID, teamID, r.PathValue("matchID"), body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
```

- [ ] **Step 12: Register the routes**

Add `Fixture *fixture.Service` to `Deps` in `internal/httpapi/router.go`, import `"pitch-ai/internal/fixture"`, and register:

```go
	if d.Auth != nil && d.Fixture != nil {
		mux.Handle("GET /api/teams/{teamID}/matches", d.protected(auth.ActionRead, d.handleListMatches))
		mux.Handle("GET /api/teams/{teamID}/matches/{matchID}", d.protected(auth.ActionRead, d.handleGetMatch))
		mux.Handle("POST /api/teams/{teamID}/matches", d.protected(auth.ActionEditMatch, d.handleCreateMatch))
		mux.Handle("PUT /api/teams/{teamID}/matches/{matchID}/lineup", d.protected(auth.ActionEditMatch, d.handleSetLineup))
	}
```

`ActionEditMatch` is held by all three roles, so a coach may create fixtures and set lineups; `ActionManageSquad` remains admin-only, which is why a coach still gets 403 on the player endpoints.

- [ ] **Step 13: Run the tests to verify they pass**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS — health, SPA, player and match tests.

- [ ] **Step 14: Wire the service in main**

In `cmd/server/main.go`, after `squadService`:

```go
	fixtureService := fixture.NewService(store, store, store, func() string { return uuid.NewString() })
```

and add `Fixture: fixtureService` to the `httpapi.Deps` literal. Import `"pitch-ai/internal/fixture"`.

- [ ] **Step 15: Regenerate types and verify everything**

Add the `internal/fixture` package to `tygo.yaml` only if it gains exported types the client needs — as written it has none, so no change is required. Run:

```bash
go build ./... && go test ./... && make types && git diff --exit-code web/src/types
```

Expected: all green. `web/src/types/core.ts` now also contains `Match`, `Lineup` and `LineupSlot`.

- [ ] **Step 16: Stage**

```bash
git add internal cmd web/src/types
```
Commit message: `feat: match scheduling, lineup selection and endpoints`

---

## Task 15: Matches and lineup UI

**Files:**
- Create: `web/src/features/matches/api.ts`, `web/src/features/matches/MatchesPage.tsx`, `web/src/features/matches/MatchForm.tsx`, `web/src/features/matches/LineupPicker.tsx`
- Modify: `web/src/main.tsx`
- Test: `web/src/features/matches/LineupPicker.test.tsx`

**Interfaces:**
- Consumes: `useAuth` (Task 11), `usePlayers` (Task 12), `core.Match`, `core.Lineup`, `core.LineupSlot` from `web/src/types/core` (Task 14), match endpoints (Task 14)
- Produces: `useMatches(teamId)`, `useCreateMatch(teamId)`, `useSetLineup(teamId)`, `<LineupPicker players lineup onSave>`

- [ ] **Step 1: Write the failing lineup test**

Create `web/src/features/matches/LineupPicker.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Player } from "../../types/core";
import { LineupPicker } from "./LineupPicker";

function squad(n: number): Player[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `player-${i + 1}`,
    clubId: "club-1",
    firstName: "First",
    lastName: `Last${i + 1}`,
    dob: "",
    positions: ["back_row"],
    teamIds: ["team-1"],
    status: "active",
  }));
}

async function fillAllFifteen() {
  for (let jersey = 1; jersey <= 15; jersey++) {
    await userEvent.selectOptions(
      screen.getByLabelText(new RegExp(`^jersey ${jersey}$`, "i")),
      `player-${jersey}`,
    );
  }
}

describe("LineupPicker", () => {
  it("saves a complete starting XV", async () => {
    const onSave = vi.fn();
    render(<LineupPicker players={squad(20)} lineup={{ starters: [], bench: [] }} onSave={onSave} />);

    await fillAllFifteen();
    await userEvent.click(screen.getByRole("button", { name: /save lineup/i }));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0][0].starters).toHaveLength(15);
  });

  it("refuses to save an incomplete XV and says how many are missing", async () => {
    const onSave = vi.fn();
    render(<LineupPicker players={squad(20)} lineup={{ starters: [], bench: [] }} onSave={onSave} />);

    await userEvent.selectOptions(screen.getByLabelText(/^jersey 1$/i), "player-1");
    await userEvent.click(screen.getByRole("button", { name: /save lineup/i }));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(/14/);
  });

  it("does not offer a player who is already selected elsewhere", async () => {
    render(<LineupPicker players={squad(20)} lineup={{ starters: [], bench: [] }} onSave={() => {}} />);

    await userEvent.selectOptions(screen.getByLabelText(/^jersey 1$/i), "player-1");

    const jerseyTwo = screen.getByLabelText(/^jersey 2$/i);
    expect(within(jerseyTwo).queryByRole("option", { name: /Last1$/ })).toBeNull();
  });
});
```

Add `within` to the import from `@testing-library/react`. Filtering already-selected players out of every other dropdown makes the "same player twice" error from Task 13 unreachable through the UI — the server still rejects it, but the coach never has to read the message.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npm test -- --run src/features/matches/LineupPicker.test.tsx`
Expected: FAIL — cannot resolve `./LineupPicker`.

- [ ] **Step 3: Implement the picker**

Create `web/src/features/matches/LineupPicker.tsx`:

```tsx
import { useState } from "react";
import type { Lineup, Player } from "../../types/core";

const STARTER_JERSEYS = Array.from({ length: 15 }, (_, i) => i + 1);
const BENCH_JERSEYS = Array.from({ length: 8 }, (_, i) => i + 16);

type Props = {
  players: Player[];
  lineup: Lineup;
  onSave: (lineup: Lineup) => void;
};

export function LineupPicker({ players, lineup, onSave }: Props) {
  const [selection, setSelection] = useState<Record<number, string>>(() => {
    const initial: Record<number, string> = {};
    for (const slot of [...(lineup.starters ?? []), ...(lineup.bench ?? [])]) {
      initial[slot.jersey] = slot.playerId;
    }
    return initial;
  });
  const [error, setError] = useState<string | null>(null);

  const taken = new Set(Object.values(selection).filter(Boolean));

  function choose(jersey: number, playerId: string) {
    setSelection((current) => {
      const next = { ...current };
      if (playerId) {
        next[jersey] = playerId;
      } else {
        delete next[jersey];
      }
      return next;
    });
  }

  function handleSave() {
    const missing = STARTER_JERSEYS.filter((jersey) => !selection[jersey]);
    if (missing.length > 0) {
      setError(`${missing.length} starting jerseys are still unfilled: ${missing.join(", ")}.`);
      return;
    }

    setError(null);
    onSave({
      starters: STARTER_JERSEYS.map((jersey) => ({ jersey, playerId: selection[jersey] })),
      bench: BENCH_JERSEYS.filter((jersey) => selection[jersey]).map((jersey) => ({
        jersey,
        playerId: selection[jersey],
      })),
    });
  }

  function slot(jersey: number) {
    const chosen = selection[jersey] ?? "";
    const options = players.filter((p) => p.id === chosen || !taken.has(p.id));

    return (
      <label key={jersey} className="flex items-center gap-2 text-sm">
        <span className="w-16 font-semibold">Jersey {jersey}</span>
        <select
          value={chosen}
          onChange={(e) => choose(jersey, e.target.value)}
          className="flex-1 rounded border border-slate-300 px-2 py-1"
        >
          <option value="">—</option>
          {options.map((p) => (
            <option key={p.id} value={p.id}>
              {p.firstName} {p.lastName}
            </option>
          ))}
        </select>
      </label>
    );
  }

  return (
    <div className="space-y-5">
      {error && (
        <p role="alert" className="rounded bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="grid gap-5 sm:grid-cols-2">
        <fieldset className="space-y-2">
          <legend className="mb-2 font-semibold">Starting XV</legend>
          {STARTER_JERSEYS.map(slot)}
        </fieldset>
        <fieldset className="space-y-2">
          <legend className="mb-2 font-semibold">Replacements</legend>
          {BENCH_JERSEYS.map(slot)}
        </fieldset>
      </div>

      <button
        type="button"
        onClick={handleSave}
        className="rounded bg-slate-900 px-4 py-2 font-semibold text-white"
      >
        Save lineup
      </button>
    </div>
  );
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npm test -- --run src/features/matches/LineupPicker.test.tsx`
Expected: PASS — three tests.

- [ ] **Step 5: Write the match hooks**

Create `web/src/features/matches/api.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../../lib/auth";
import type { Lineup, Match } from "../../types/core";

const matchesKey = (teamId: string) => ["matches", teamId];

export function useMatches(teamId: string) {
  const { apiFetch } = useAuth();
  return useQuery({
    queryKey: matchesKey(teamId),
    queryFn: () => apiFetch<Match[]>(`/teams/${teamId}/matches`),
    enabled: Boolean(teamId),
  });
}

export function useCreateMatch(teamId: string) {
  const { apiFetch } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (match: Match) =>
      apiFetch<Match>(`/teams/${teamId}/matches`, {
        method: "POST",
        body: JSON.stringify(match),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: matchesKey(teamId) }),
  });
}

export function useSetLineup(teamId: string) {
  const { apiFetch } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ matchId, lineup }: { matchId: string; lineup: Lineup }) =>
      apiFetch<Match>(`/teams/${teamId}/matches/${matchId}/lineup`, {
        method: "PUT",
        body: JSON.stringify(lineup),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: matchesKey(teamId) }),
  });
}
```

- [ ] **Step 6: Write the match form**

Create `web/src/features/matches/MatchForm.tsx`:

```tsx
import { type FormEvent, useState } from "react";
import type { Match, Venue } from "../../types/core";

const VENUE_LABELS: Record<Venue, string> = {
  home: "Home",
  away: "Away",
  neutral: "Neutral",
};

type Props = {
  onSubmit: (match: Match) => void;
  onCancel: () => void;
};

export function MatchForm({ onSubmit, onCancel }: Props) {
  const [opponent, setOpponent] = useState("");
  const [kickoff, setKickoff] = useState("");
  const [competition, setCompetition] = useState("");
  const [seasonId, setSeasonId] = useState("");
  const [venue, setVenue] = useState<Venue>("home");
  const [error, setError] = useState<string | null>(null);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();

    if (!opponent.trim() || !kickoff || !seasonId.trim()) {
      setError("Opponent, kick-off and season are required.");
      return;
    }

    setError(null);
    onSubmit({
      id: "",
      clubId: "",
      teamId: "",
      seasonId: seasonId.trim(),
      opponent: opponent.trim(),
      kickoffAt: new Date(kickoff).toISOString(),
      competition: competition.trim(),
      venue,
      status: "scheduled",
      lineup: { starters: [], bench: [] },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 rounded-xl bg-white p-5 shadow-sm">
      {error && (
        <p role="alert" className="rounded bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Opponent</span>
          <input
            value={opponent}
            onChange={(e) => setOpponent(e.target.value)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Kick-off</span>
          <input
            type="datetime-local"
            value={kickoff}
            onChange={(e) => setKickoff(e.target.value)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Competition</span>
          <input
            value={competition}
            onChange={(e) => setCompetition(e.target.value)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Season</span>
          <input
            value={seasonId}
            onChange={(e) => setSeasonId(e.target.value)}
            placeholder="2026-27"
            className="w-full rounded border border-slate-300 px-3 py-2"
          />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block font-medium">Venue</span>
          <select
            value={venue}
            onChange={(e) => setVenue(e.target.value as Venue)}
            className="w-full rounded border border-slate-300 px-3 py-2"
          >
            {Object.entries(VENUE_LABELS).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="flex gap-3">
        <button type="submit" className="rounded bg-slate-900 px-4 py-2 font-semibold text-white">
          Save
        </button>
        <button type="button" onClick={onCancel} className="rounded px-4 py-2 text-slate-600">
          Cancel
        </button>
      </div>
    </form>
  );
}
```

- [ ] **Step 7: Write the matches page**

Create `web/src/features/matches/MatchesPage.tsx`:

```tsx
import { useState } from "react";
import { useAuth } from "../../lib/auth";
import { usePlayers } from "../players/api";
import { LineupPicker } from "./LineupPicker";
import { MatchForm } from "./MatchForm";
import { useCreateMatch, useMatches, useSetLineup } from "./api";

export function MatchesPage() {
  const { membership } = useAuth();
  const teamId = membership?.teamIds?.[0] ?? "";

  const { data: matches, isPending } = useMatches(teamId);
  const { data: players } = usePlayers();
  const createMatch = useCreateMatch(teamId);
  const setLineup = useSetLineup(teamId);

  const [adding, setAdding] = useState(false);
  const [selectingFor, setSelectingFor] = useState<string | null>(null);

  const canEdit = membership?.role !== undefined;
  const openMatch = matches?.find((m) => m.id === selectingFor);

  if (!teamId) return <p className="text-red-700">Your account has no team assigned.</p>;
  if (isPending) return <p className="text-slate-500">Loading fixtures…</p>;

  return (
    <section className="space-y-5">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Matches</h1>
        {canEdit && !adding && (
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="rounded bg-slate-900 px-4 py-2 text-sm font-semibold text-white"
          >
            Add match
          </button>
        )}
      </div>

      {adding && (
        <MatchForm
          onSubmit={(match) =>
            createMatch.mutate(match, { onSuccess: () => setAdding(false) })
          }
          onCancel={() => setAdding(false)}
        />
      )}

      <ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
        {matches?.map((match) => (
          <li key={match.id} className="flex items-center gap-4 px-5 py-3">
            <div className="flex-1">
              <p className="font-medium">
                {match.venue === "home" ? "vs" : "at"} {match.opponent}
              </p>
              <p className="text-sm text-slate-500">
                {new Date(match.kickoffAt).toLocaleString()} · {match.competition} ·{" "}
                {match.lineup?.starters?.length === 15 ? "XV selected" : "no lineup"}
              </p>
            </div>
            {canEdit && (
              <button
                type="button"
                onClick={() => setSelectingFor(match.id)}
                className="text-sm text-slate-600"
              >
                Select XV
              </button>
            )}
          </li>
        ))}
        {matches?.length === 0 && <li className="px-5 py-6 text-slate-500">No matches yet.</li>}
      </ul>

      {openMatch && players && (
        <div className="rounded-xl bg-white p-5 shadow-sm">
          <h2 className="mb-4 font-semibold">Lineup — {openMatch.opponent}</h2>
          <LineupPicker
            players={players.filter((p) => p.status === "active" && p.teamIds.includes(teamId))}
            lineup={openMatch.lineup}
            onSave={(lineup) =>
              setLineup.mutate(
                { matchId: openMatch.id, lineup },
                { onSuccess: () => setSelectingFor(null) },
              )
            }
          />
          {setLineup.isError && (
            <p role="alert" className="mt-3 text-sm text-red-700">
              {(setLineup.error as Error).message}
            </p>
          )}
        </div>
      )}
    </section>
  );
}
```

- [ ] **Step 8: Add the route**

In `web/src/main.tsx`, import `MatchesPage` and add to the shell's `children`:

```tsx
      { path: "matches", element: <MatchesPage /> },
```

- [ ] **Step 9: Verify in the browser**

With both servers running, create a match, open "Select XV", fill all 15 jerseys plus a few replacements, and save. Reload and confirm it shows "XV selected".
Expected: the lineup persists, and an incomplete selection is refused before any request is sent.

- [ ] **Step 10: Run checks**

Run: `cd web && npx biome ci . && npx tsc --noEmit && npm test -- --run`
Expected: all pass.

- [ ] **Step 11: Stage**

```bash
git add web
```
Commit message: `feat(web): match list, match form and lineup picker`

---

## Task 16: Deploy M1 and enable continuous deployment

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the Terraform outputs and `make deploy` (Task 5), the passing CI jobs (Task 6)

- [ ] **Step 1: Run the whole suite one more time**

```bash
go build ./... && go test ./... -race
make types && git diff --exit-code web/src/types
cd web && npx biome ci . && npx tsc --noEmit && npm test -- --run && npm run build
```

Expected: everything passes, no generated-type drift.

- [ ] **Step 2: Run the store tests against the emulator**

In one terminal `make emulator`, in another `make test-store`.
Expected: PASS — membership, player and match suites.

- [ ] **Step 3: Deploy**

```bash
make deploy
```

- [ ] **Step 4: Verify the deployed application by hand**

Open the Cloud Run URL. Sign in with Google. Confirm: the squad list loads, adding a player persists across a reload, creating a match works, and selecting a full XV saves. Then open the browser console and confirm there are no CSP or network errors.

Expected: every M1 flow works against the real Firestore, not just the emulator.

- [ ] **Step 5: Add the deploy job**

Now that a manual deploy has been proven, append to `.github/workflows/ci.yml`:

```yaml
  deploy:
    needs: [go, web]
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v4
      - uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: ${{ vars.WIF_PROVIDER }}
          service_account: ${{ vars.DEPLOY_SERVICE_ACCOUNT }}
      - uses: google-github-actions/setup-gcloud@v2
      - run: gcloud builds submit --tag ${{ vars.IMAGE }}:${{ github.sha }} .
      - run: |
          gcloud run deploy ${{ vars.SERVICE_NAME }} \
            --image ${{ vars.IMAGE }}:${{ github.sha }} \
            --region ${{ vars.REGION }}
```

Set the repository variables `WIF_PROVIDER`, `DEPLOY_SERVICE_ACCOUNT`, `IMAGE`, `SERVICE_NAME` and `REGION` under Settings → Secrets and variables → Actions → Variables. `IMAGE` is the `image_repository` Terraform output. Workload Identity Federation is used rather than a service-account key so no long-lived credential is stored in GitHub.

- [ ] **Step 6: Stage**

```bash
git add .github
```
Commit message: `ci: deploy to Cloud Run on merge to main`

---

## Definition of done

M0 and M1 are complete when all of the following are true:

1. `go test ./... -race` passes, and `make test-store` passes against the emulator.
2. `npx biome ci . && npx tsc --noEmit && npm test -- --run` pass in `web/`.
3. `make types` produces no diff.
4. The deployed Cloud Run URL serves the PWA, authenticates with Google, and refuses an account with no club membership with a clear message.
5. A coach can be added, edited and removed from the squad; a match can be created; a full XV plus replacements can be selected and survives a reload.
6. A `coach`-role account can read the squad and manage fixtures, but cannot add, edit or remove players.

## What M2 will build on this

M2 (live tagging) is the next plan and depends on this one for: the `core` package conventions and `ValidationError`, the auth middleware and `Deps.protected` helper, the Firestore `clubDoc` path helper, the generated-types pipeline, and `Match.Lineup` — which is what makes minutes-played computable once events start arriving.
