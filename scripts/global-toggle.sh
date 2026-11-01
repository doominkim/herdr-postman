#!/bin/sh
# Requires the Herdr build with plugin.runtime.toggle support.
# Visibility changes never navigate to, create, or close a repository tab.
set -eu
herdr=${HERDR_BIN_PATH:-herdr}
app_dir="${XDG_DATA_HOME:-$HOME/.local/share}/herdr-postman"
mkdir -p "$app_dir"
exec "$herdr" plugin runtime toggle --plugin herdr-postman --entrypoint postman --cwd "$app_dir"
