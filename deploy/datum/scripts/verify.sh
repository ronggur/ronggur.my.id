#!/usr/bin/env bash
# Verify the deploy from outside — the platform has no probes and no logs
# (register rows L5, L6), so curl is the only health signal there is.
#
#   deploy/datum/scripts/verify.sh
#
# Checks every instance directly at externalIP:PORT (a 1:1 NAT to the
# instance), then the Gateway's canonical hostname if the Gateway exists.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

need datumctl
need jq

rc=0

info "instances"
datumctl compute instances --workload "$WORKLOAD" || true

ADDRS="$(datumctl compute instances --workload "$WORKLOAD" -o json \
  | jq -r '[.items[].status.networkInterfaces[0].assignments.externalIP | select(. != null and . != "")] | unique | .[]' || true)"

if [ -z "$ADDRS" ]; then
  printf '\033[33mwarning:\033[0m no instance external IPs yet\n' >&2
  rc=1
else
  while IFS= read -r ip; do
    info "curl http://$ip:$PORT/healthz"
    if curl -fsS --max-time 10 "http://$ip:$PORT/healthz"; then :; else
      printf '\033[31mfail:\033[0m %s did not answer\n' "$ip" >&2; rc=1
    fi
  done <<< "$ADDRS"
fi

if command -v kubectl >/dev/null 2>&1; then
  HOST="$(kubectl get gateway "$GATEWAY" -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)"
  if [ -n "$HOST" ]; then
    info "curl https://$HOST/"
    if curl -fsS --max-time 15 -o /dev/null -w 'HTTP %{http_code} in %{time_total}s\n' "https://$HOST/"; then :; else
      printf '\033[31mfail:\033[0m %s did not answer — a stale EndpointSlice is the first suspect (L8)\n' "$HOST" >&2; rc=1
    fi
  else
    info "no Gateway address yet — skipping the hostname check"
  fi
fi

exit $rc
