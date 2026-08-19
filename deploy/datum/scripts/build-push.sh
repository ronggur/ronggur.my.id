#!/usr/bin/env bash
# Build the unikernel image and push it to a registry cells can pull from
# without per-workload credentials (register row L9), then report the digest.
#
#   deploy/datum/scripts/build-push.sh [tag]
#
# Requires: kraft, a running container runtime with BuildKit (the rootfs stage
# builds with it), crane, and a registry login for $IMAGE.
#
# Also requires `kraft login` first: the base-compat runtime lives on
# index.unikraft.io, which denies anonymous pulls, and access to the base is
# itself gated (register row L51). Without it the run stops at
#   could not find runtime 'index.unikraft.io/official/base-compat:latest'
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

need kraft
need crane

TAG="${1:-$(git -C "$REPO_ROOT" rev-parse --short HEAD)}"
REF="$IMAGE:$TAG"

docker info >/dev/null 2>&1 || \
  die "no container runtime reachable — kraft builds the rootfs with BuildKit and fails before it starts"

info "packaging $REF from $REPO_ROOT/Kraftfile"
cd "$REPO_ROOT"
# --plat kraftcloud --arch x86_64 is what puts the kraftcloud/x86_64 platform
# entry kraftlet requires into the pushed OCI index. The handbook never
# reproduced this build, so treat the first run as the experiment it is.
if ! kraft pkg --name "$REF" --plat kraftcloud --arch x86_64 --push .; then
  die "kraft pkg failed — if it could not find the base-compat runtime, run 'kraft login' (register row L51)"
fi

info "verifying the pushed index carries kraftcloud/x86_64"
if ! crane manifest "$REF" | jq -e '
      (.manifests // []) | map(.platform | "\(.os)/\(.architecture)") | index("kraftcloud/x86_64")
    ' >/dev/null 2>&1; then
  printf '\033[33mwarning:\033[0m no kraftcloud/x86_64 entry found in %s — kraftlet may refuse it\n' "$REF" >&2
  crane manifest "$REF" | jq -c '.manifests // [] | map(.platform)' >&2 || true
fi

DIGEST="$(crane digest "$REF")"
info "pushed $REF"
info "digest $DIGEST"
cat <<PIN

Pin it into the production overlay:

  cd $DATUM_DIR/overlays/production
  kustomize edit set image "$IMAGE=$IMAGE@$DIGEST"

PIN
