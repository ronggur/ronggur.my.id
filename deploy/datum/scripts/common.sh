# Shared settings for the Datum Compute scripts. Override any of these in the
# environment: IMAGE=ghcr.io/you/app deploy/datum/scripts/build-push.sh
IMAGE="${IMAGE:-ghcr.io/ronggur/ronggur-my-id}"
WORKLOAD="${WORKLOAD:-ronggur-my-id}"
PORT="${PORT:-8080}"
SLICE="${SLICE:-ronggur-my-id-endpoints}"
GATEWAY="${GATEWAY:-ronggur-my-id-gw}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DATUM_DIR="$REPO_ROOT/deploy/datum"

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 not found in PATH"; }
