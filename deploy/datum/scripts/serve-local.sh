#!/usr/bin/env bash
# Run the same server the unikernel runs, against the working tree, so what
# you test locally is the code that ships.
#
#   deploy/datum/scripts/serve-local.sh [port]
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

need go
LOCAL_PORT="${1:-$PORT}"

info "serving $REPO_ROOT on http://127.0.0.1:$LOCAL_PORT (Ctrl-C to stop)"
cd "$DATUM_DIR/server"
PORT="$LOCAL_PORT" SITE_ROOT="$REPO_ROOT" go run .
