#!/bin/sh
# Separate Postman pane lifecycle; switching back leaves the app running.
set -eu

herdr=${HERDR_BIN_PATH:-herdr}
ctx=${HERDR_PLUGIN_CONTEXT_JSON:-'{}'}
state_dir=${HERDR_POSTMAN_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/herdr-postman}
jq=/usr/bin/jq
[ -x "$jq" ] || jq=jq

field() { printf '%s' "$ctx" | "$jq" -r "$1 // empty"; }

workspace=$(field .workspace_id)
case "$workspace" in ''|*[!a-zA-Z0-9:_-]*) printf '%s\n' 'Invalid Herdr workspace context.' >&2; exit 2;; esac
focused=$(field .focused_pane_id)
panes=$("$herdr" pane list | "$jq" -c '.result.panes')

posting=$(printf '%s' "$panes" | "$jq" -r --arg focused "$focused" '[.[] | select(.label == "herdr-postman") | .pane_id] | (map(select(. == $focused)) + .) | first // empty')
posting_tab=$(printf '%s' "$panes" | "$jq" -r --arg id "$posting" '[.[] | select(.pane_id == $id) | .tab_id] | first // empty')
state_file="$state_dir/global.tab"

remember_tab() {
  tab=$(printf '%s' "$panes" | "$jq" -r --arg id "$focused" '[.[] | select(.pane_id == $id) | .tab_id] | first // empty')
  [ -n "$tab" ] || return 0
  mkdir -p "$state_dir"
  printf '%s\n' "$tab" >"$state_file"
}

if [ -n "$posting" ] && [ "$posting" = "$focused" ]; then
  others=$(printf '%s' "$panes" | "$jq" -r --arg tab "$posting_tab" '[.[] | select(.label == "herdr-postman") | .tab_id] as $postman_tabs | .[] | select(.tab_id as $id | $postman_tabs | index($id) | not) | .tab_id')
  back=$(cat "$state_file" 2>/dev/null || :)
  if [ -z "$back" ] || ! printf '%s\n' "$others" | grep -Fxq "$back"; then
    back=$(printf '%s\n' "$others" | head -n 1)
  fi
  [ -n "$back" ] || exit 0
  exec "$herdr" tab focus "$back"
fi

remember_tab

# Focusing a pane in a hidden tab doesn't show that tab, so show the tab first.
if [ -n "$posting" ]; then
  "$herdr" tab focus "$posting_tab" >/dev/null
  exec "$herdr" plugin pane focus "$posting"
fi

set -- plugin pane open --plugin herdr-postman --entrypoint postman --placement tab --focus

# The app belongs to the user, not the repository invoking F8.
app_dir="${XDG_DATA_HOME:-$HOME/.local/share}/herdr-postman"
mkdir -p "$app_dir"
set -- "$@" --cwd "$app_dir"

opened_tab=$("$herdr" "$@" | "$jq" -r '.result.plugin_pane.pane.tab_id // empty')
[ -n "$opened_tab" ] || exit 1
exec "$herdr" tab focus "$opened_tab"
