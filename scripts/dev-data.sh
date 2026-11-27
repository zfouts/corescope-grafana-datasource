#!/usr/bin/env bash
# Build dev/data/meshcore.db: CoreScope's public e2e fixture, timestamps shifted
# to now, plus generated observer_metrics for the 3 busiest observers.
# Idempotent: re-running rebuilds meshcore.db from the pristine download.
# Stop the corescope container first if it is running (it holds the DB open).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="$ROOT/dev/data"
TAG=v3.18.0
URL="https://raw.githubusercontent.com/Kpa-clawbot/CoreScope/$TAG/test-fixtures/e2e-fixture.db"
SHA256=8ac2681b6254fea010ca815d27355b8e861b09399faaa700041e7f6c83c16245
ORIG="$DATA/fixture.orig.db"

mkdir -p "$DATA"
if [ ! -f "$ORIG" ] || ! echo "$SHA256  $ORIG" | sha256sum -c --status; then
  curl -fsSL "$URL" -o "$ORIG.tmp"
  echo "$SHA256  $ORIG.tmp" | sha256sum -c --status || { rm -f "$ORIG.tmp"; echo "fixture sha256 mismatch" >&2; exit 1; }
  mv "$ORIG.tmp" "$ORIG"
fi

rm -f "$DATA/meshcore.db" "$DATA/meshcore.db-wal" "$DATA/meshcore.db-shm"
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$ROOT/scripts:/work/scripts:ro" -v "$DATA:/work/data" \
  python:3.13-alpine python -I /work/scripts/dev_seed.py /work/data/fixture.orig.db /work/data/meshcore.db
chmod a+rw "$DATA/meshcore.db" "$DATA"
