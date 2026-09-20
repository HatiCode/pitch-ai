# syntax=docker/dockerfile:1
# check=skip=SecretsUsedInArgOrEnv
#
# The skip is deliberate and narrow: the linter matches on names containing KEY
# and AUTH, but the Firebase web config is public by design — Vite inlines it
# into the client bundle. It is passed as a build arg to keep it out of the
# repository, not to keep it confidential.

FROM node:26-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./

# Public once served, but injected rather than committed — see cloudbuild.yaml.
ARG VITE_FIREBASE_API_KEY
ARG VITE_FIREBASE_AUTH_DOMAIN
ARG VITE_FIREBASE_PROJECT_ID
ENV VITE_FIREBASE_API_KEY=$VITE_FIREBASE_API_KEY \
	VITE_FIREBASE_AUTH_DOMAIN=$VITE_FIREBASE_AUTH_DOMAIN \
	VITE_FIREBASE_PROJECT_ID=$VITE_FIREBASE_PROJECT_ID

# Vite silently inlines `undefined` for a missing variable, which would deploy
# an app that only fails at sign-in. Fail the build instead.
RUN test -n "$VITE_FIREBASE_API_KEY" && test -n "$VITE_FIREBASE_PROJECT_ID" \
	|| (echo "ERROR: Firebase build args missing; see cloudbuild.yaml" >&2 && exit 1)

RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The web stage's output replaces the placeholder dist/ committed for go:embed.
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
