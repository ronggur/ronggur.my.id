#!/usr/bin/env bash
# Render the production overlay and deploy it.
#
#   deploy/datum/scripts/deploy.sh
#
# Always deploys with -f. The flags form (deploy <name> --image ...) rebuilds
# the whole spec from flags and would wipe the env, ports, and network policy
# in workload.yaml (register row L54). deploy -f cannot read stdin
# (register row L53), hence the real file.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

need datumctl

# Standalone kustomize if present, otherwise the one built into kubectl.
if command -v kustomize >/dev/null 2>&1; then
  render() { kustomize build "$1"; }
elif command -v kubectl >/dev/null 2>&1; then
  render() { kubectl kustomize "$1"; }
else
  die "neither kustomize nor kubectl found in PATH"
fi

RENDERED="$REPO_ROOT/workload.yaml"

info "rendering overlays/production -> $RENDERED"
render "$DATUM_DIR/overlays/production" > "$RENDERED"
grep -q 'image: .*@sha256:' "$RENDERED" || \
  printf '\033[33mwarning:\033[0m image is not digest-pinned — a mutable tag re-push rolls nothing (L52)\n' >&2

info "deploying $WORKLOAD"
# The command blocks watching the rollout and has no flag to skip the watch;
# Ctrl-C detaches safely and the rollout continues server-side.
datumctl compute deploy -f "$RENDERED" -y

info "next: deploy/datum/scripts/repoint-ingress.sh, then verify.sh"
