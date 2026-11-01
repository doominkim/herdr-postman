#!/bin/sh
set -eu
plugin_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$plugin_root"
if ! command -v go >/dev/null 2>&1; then
  printf '%s\n' 'Go 1.25.5 or later is required to build herdr-postman.' >&2
  exit 127
fi
exec go build -o bin/herdr-postman ./cmd/herdr-postman
