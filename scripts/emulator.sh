#!/usr/bin/env bash
set -euo pipefail

# Firestore emulator for local store tests.
#
# Runs in Docker because the emulator is a Java application and we deliberately
# keep a JDK off the host. The image bundles both the JRE and the emulator.
#
#   ./scripts/emulator.sh          start in the foreground (Ctrl-C to stop)
#   make test-store                run the store tests against it

IMAGE="gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators"
NAME="pitch-ai-firestore"
PORT="${FIRESTORE_EMULATOR_PORT:-8081}"

# Remove a container left behind by an earlier interrupted run.
docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "Firestore emulator listening on localhost:${PORT}"
echo "Run tests with: make test-store"

exec docker run --rm --name "$NAME" -p "${PORT}:${PORT}" "$IMAGE" \
	gcloud emulators firestore start --host-port="0.0.0.0:${PORT}"
