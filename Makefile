.PHONY: test test-go test-web test-store build web run fmt types emulator deploy

REGION ?= europe-west1
SERVICE ?= pitch-ai
PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
IMAGE = $(REGION)-docker.pkg.dev/$(PROJECT)/$(SERVICE)/$(SERVICE)

test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && npm test -- --run

# Store tests need the Firestore emulator; run `make emulator` in another shell.
test-store:
	FIRESTORE_EMULATOR_HOST=localhost:8081 go test ./internal/store/... -count=1

emulator:
	./scripts/emulator.sh

web:
	cd web && npm ci && npm run build

build: web
	go build -o bin/server ./cmd/server

run: build
	./bin/server

fmt:
	go fmt ./...

# tygo generates the struct shapes; gen-types narrows the string enums into
# unions and emits the position-to-group table.
types:
	go run github.com/gzuidhof/tygo@v0.2.21 generate
	go run ./cmd/gen-types
	cd web && npx biome format --write src/types >/dev/null

# Cloud Build produces a linux/amd64 image; a local `docker build` on Apple
# Silicon would produce arm64, which Cloud Run refuses to start.
#
# The Firebase web config is read from web/.env.local (gitignored) and passed as
# build substitutions, so it never lives in the repository. CI supplies the same
# values from GitHub Actions secrets.
deploy:
	@test -f web/.env.local || (echo "web/.env.local missing; copy web/.env.example and fill it in" >&2 && exit 1)
	set -a && . ./web/.env.local && set +a && \
	gcloud builds submit --config cloudbuild.yaml \
		--substitutions=_IMAGE=$(IMAGE):latest,_FIREBASE_API_KEY=$$VITE_FIREBASE_API_KEY,_FIREBASE_AUTH_DOMAIN=$$VITE_FIREBASE_AUTH_DOMAIN,_FIREBASE_PROJECT_ID=$$VITE_FIREBASE_PROJECT_ID \
		.
	gcloud run deploy $(SERVICE) --image $(IMAGE):latest --region $(REGION)
