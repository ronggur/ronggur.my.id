# status.ronggur.my.id

A small status page in a single static Go binary with no external
dependencies. Besides showing status, it serves as a Datum test case (Compute,
ALB, DNS), so failures are shown as they are, on purpose.

Deploy: `../docs/deploy-status.md`.

## Run locally

```bash
cd status
PORT=8081 PROBE_INTERVAL=5 go run .
go vet ./... && go test ./...
```

## Endpoints

| Path | Content |
|---|---|
| `/` | Status page (HTML, embedded) |
| `/api/status` | JSON: status, uptime, requests, instance info, probe results and history |
| `/healthz` | `ok`. Never redirected and not counted as a request |

GET and HEAD only. The HTTP to HTTPS redirect and HSTS work through
`X-Forwarded-Proto`, the same as `../server/main.go`.

## Probes

Run every `PROBE_INTERVAL`, each limited by `PROBE_TIMEOUT`.

| Probe | Checks | Env | Critical |
|---|---|---|---|
| `self` | `/healthz` handler called in process, no socket (the unikernel runtime has no loopback) | - | yes |
| `dns` | resolves a few hostnames | `PROBE_DNS_HOSTS` | no |
| `http` | `GET` returns 2xx (a redirect counts as a failure) | `PROBE_HTTP_URL` | no |
| `google` | `GET` to Google returns 2xx: a well-known outside host, to tell "the internet is unreachable" from "only my site is" | `PROBE_GOOGLE_URL` | no |
| `tls` | handshake and remaining certificate lifetime | `PROBE_TLS_ADDR` | no |
| `outbound` | TCP to a literal IP, no DNS | `PROBE_OUTBOUND_ADDR` | no |

Status: `down` if `self` fails, `degraded` if any other probe fails,
otherwise `operational`. Probes that have not run yet are ignored.

`outbound` uses a literal IP so "no outbound" can be told apart from "DNS is
broken". Datum Compute has IPv6 egress only: IPv4 literals fail with
`network is unreachable`, so the default target is an IPv6 address. On the
unikernel runtime the instance has no loopback and no `/etc/resolv.conf`, and
the first seconds after boot have no route yet; the probes above handle all
three (in-process `self`, `DNS_FALLBACK`, and a bounded wait before round one).

The `dns` card lists `PROBE_DNS_HOSTS` verbatim. Set it per workload to the
hostname that workload is served on, for example
`PROBE_DNS_HOSTS=ronggur.my.id,status-gp.ronggur.my.id`.

| Env | Default |
|---|---|
| `PORT` | `8080` |
| `PROBE_INTERVAL` | `30s` (a bare number = seconds) |
| `PROBE_TIMEOUT` | `5s` |
| `PROBE_DNS_HOSTS` | `ronggur.my.id,status.ronggur.my.id` |
| `PROBE_HTTP_URL` | `https://ronggur.my.id/healthz` |
| `PROBE_GOOGLE_URL` | `https://www.google.com/generate_204` |
| `PROBE_TLS_ADDR` | `ronggur.my.id:443` |
| `PROBE_OUTBOUND_ADDR` | `[2606:4700:4700::1111]:443` (the platform has no IPv4 egress) |
| `DNS_FALLBACK` | `[2001:4860:4860::6464]:53`, used only when `/etc/resolv.conf` names no server (unikernel) |
| `REGION` | empty |

## Deliberate limitations

- History (120 samples per probe) lives in memory only. It is lost on restart,
  and each instance has its own.
- The instance region is looked up from `REGION`, then from the instance
  hostname. Placements share one template, so the env cannot differ per
  region. With no hint, the page says "unknown".
- Only an allowlist of headers is shown (`X-Forwarded-*`, `X-Request-Id`,
  `Via`), and the page renders them as text, not HTML.
- The `dns` probe for `status.ronggur.my.id` fails until the subdomain is set
  up.
