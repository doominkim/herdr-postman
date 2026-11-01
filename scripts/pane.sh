#!/bin/sh
# Launch the separate Postman application with Cloud/Local source selection.
set -eu
plugin_root=${HERDR_PLUGIN_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
find_app() {
  if [ -n "${HERDR_POSTMAN_BIN:-}" ]; then printf '%s' "$HERDR_POSTMAN_BIN"; return; fi
  for candidate in "$plugin_root/bin/herdr-postman" "$(command -v herdr-postman 2>/dev/null || :)" "$HOME/.local/bin/herdr-postman"; do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then printf '%s' "$candidate"; return; fi
  done
}
bin=$(find_app)
if [ -z "$bin" ] || [ ! -x "$bin" ]; then
  printf '%s\n' 'herdr-postman binary not found. Build with: go build -o bin/herdr-postman ./cmd/herdr-postman' >&2
  exit 127
fi
postman=${POSTMAN_BIN:-$(command -v postman 2>/dev/null || :)}
if [ -z "$postman" ] && [ -x "$HOME/.local/bin/postman" ]; then postman=$HOME/.local/bin/postman; fi
if [ -z "$postman" ] || [ ! -x "$postman" ]; then
  printf '%s\n' 'Postman CLI not found. Set POSTMAN_BIN to its executable path.' >&2
  exit 127
fi
collection=${HERDR_POSTMAN_COLLECTION:-}
if [ -z "$collection" ]; then
  collection="${XDG_DATA_HOME:-$HOME/.local/share}/herdr-postman/postman/collections/default"
  mkdir -p "$collection/.resources"
  if [ ! -e "$collection/.resources/definition.yaml" ]; then
    (set -C; printf '%s\n' '$kind: collection' 'name: Personal requests' >"$collection/.resources/definition.yaml") 2>/dev/null || [ -f "$collection/.resources/definition.yaml" ]
  fi
fi
if [ -n "$collection" ] && [ ! -d "$collection" ]; then printf '%s\n' 'HERDR_POSTMAN_COLLECTION is not a directory.' >&2; exit 2; fi
if [ -n "$collection" ]; then
  set -- --collection "$collection" --postman-bin "$postman"
else
  set -- --postman-bin "$postman"
fi
if [ -n "${HERDR_POSTMAN_ENVIRONMENT:-}" ]; then set -- "$@" --environment "$HERDR_POSTMAN_ENVIRONMENT"; fi
set -- "$@" --source choose
exec "$bin" "$@"
