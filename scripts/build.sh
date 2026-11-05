#!/bin/sh
# Development only: no su, installation, device restart, or runtime policy changes.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$ROOT/.gotmp" "$ROOT/module/bin"
export TMPDIR="$ROOT/.gotmp" GOTMPDIR="$ROOT/.gotmp"
export GOMAXPROCS="${GOMAXPROCS:-2}"
cd "$ROOT/src"
if [ "$(go env GOOS)/$(go env GOARCH)" != android/arm64 ]; then
  printf '%s\n' 'Build on Android arm64 (Termux Go), or provide a separately verified Android toolchain.' >&2
  exit 2
fi
go test -p 1 ./...
CGO_ENABLED=0 go build -buildmode=pie -trimpath -tags netgo,osusergo \
  -ldflags='-s -w -buildid=' -o "$ROOT/module/bin/guardian" .
exec python3 "$ROOT/scripts/package.py"
