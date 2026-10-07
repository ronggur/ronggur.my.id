# status.ronggur.my.id

Status page kecil dalam satu binary Go statis tanpa dependensi luar. Selain
menampilkan status, ia dipakai sebagai test case Datum (Compute, ALB, DNS),
jadi kegagalan sengaja ditampilkan apa adanya.

Deploy: `../docs/deploy-status.md`.

## Jalan lokal

```bash
cd status
PORT=8081 PROBE_INTERVAL=5 go run .
go vet ./... && go test ./...
```

## Endpoint

| Path | Isi |
|---|---|
| `/` | Halaman status (HTML, di-embed) |
| `/api/status` | JSON: status, uptime, request, info instance, hasil dan riwayat probe |
| `/healthz` | `ok`. Tidak pernah di-redirect dan tidak dihitung sebagai request |

Hanya GET dan HEAD. Redirect HTTP ke HTTPS dan HSTS bekerja lewat
`X-Forwarded-Proto`, sama seperti `../server/main.go`.

## Probe

Jalan tiap `PROBE_INTERVAL`, masing-masing dibatasi `PROBE_TIMEOUT`.

| Probe | Memeriksa | Env | Kritis |
|---|---|---|---|
| `self` | `GET 127.0.0.1:$PORT/healthz` | - | ya |
| `dns` | resolve beberapa hostname | `PROBE_DNS_HOSTS` | tidak |
| `http` | `GET` 2xx (redirect dianggap gagal) | `PROBE_HTTP_URL` | tidak |
| `tls` | handshake dan sisa umur sertifikat | `PROBE_TLS_ADDR` | tidak |
| `outbound` | TCP ke IP literal, tanpa DNS | `PROBE_OUTBOUND_ADDR` | tidak |

Status: `down` bila `self` gagal, `degraded` bila probe lain gagal,
selain itu `operational`. Probe yang belum pernah jalan diabaikan.

`outbound` memakai IP literal supaya "tidak ada outbound" bisa dibedakan
dari "DNS rusak". Compute preview belum punya outbound internet, jadi
probe luar diharapkan gagal di Datum dan lolos di laptop.

| Env | Default |
|---|---|
| `PORT` | `8080` |
| `PROBE_INTERVAL` | `30s` (angka polos = detik) |
| `PROBE_TIMEOUT` | `5s` |
| `PROBE_DNS_HOSTS` | `ronggur.my.id,status.ronggur.my.id` |
| `PROBE_HTTP_URL` | `https://ronggur.my.id/healthz` |
| `PROBE_TLS_ADDR` | `ronggur.my.id:443` |
| `PROBE_OUTBOUND_ADDR` | `1.1.1.1:443` |
| `REGION` | kosong |

## Batasan yang disengaja

- Riwayat (120 sampel per probe) hanya di memori. Hilang saat restart, dan
  tiap instance punya riwayat sendiri.
- Region instance dicari dari `REGION`, lalu dari hostname instance. Placement
  berbagi satu template, jadi env tidak bisa berbeda per region. Bila tidak
  ada petunjuk, page menulis "tidak diketahui".
- Header yang ditampilkan hanya allowlist (`X-Forwarded-*`, `X-Request-Id`,
  `Via`), dan halaman memakainya sebagai teks, bukan HTML.
- Probe `dns` untuk `status.ronggur.my.id` gagal sampai subdomainnya dipasang.
