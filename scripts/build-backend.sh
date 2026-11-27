#!/usr/bin/env bash
# Build the Go backend into dist/ with mage inside a golang container (no local Go needed).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGETS="build:linux"
case "$(uname -m)" in aarch64|arm64) TARGETS="$TARGETS build:linuxARM64" ;; esac

mkdir -p "$ROOT/dist"
docker run --rm \
  -v "$ROOT:/src" -v cs-gomod:/go/pkg/mod -w /src \
  -e UID_GID="$(id -u):$(id -g)" \
  golang:1.27 sh -c "
    set -e
    git config --global --add safe.directory /src
    for t in $TARGETS; do go run github.com/magefile/mage@v1.17.2 -v \$t; done
    chown -R \"\$UID_GID\" /src/dist
  "
