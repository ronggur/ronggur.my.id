# Deploy status.ronggur.my.id

Page status di `status/` (lihat `status/README.md`). Workload terpisah dari
situs utama: `status-ronggur-my-id`, project `personal-project-86e0525b`,
network `ronggur-my-id` yang sama. Tidak ada resource situs utama yang
disentuh. Semua langkah di bawah dijalankan manual.

## Image

Actions → **publish-status-image** (menjalankan `go vet` dan `go test` dulu):

```bash
gh workflow run publish-status-image.yml --ref main
gh run watch "$(gh run list --workflow=publish-status-image.yml --limit 1 --json databaseId -q '.[0].databaseId')" --exit-status
```

Tag default `status-<sha pendek>`, digest ada di summary run. Setelah push
pertama, package `status-ronggur-my-id` di GHCR harus dibuat **publik**
(Package settings → Change visibility), kalau tidak Datum tidak bisa menarik
image-nya.

Cek media type, harus `application/vnd.docker.distribution.manifest.v2+json`:

```bash
TAG=status-$(git rev-parse --short=7 HEAD)
T=$(curl -s "https://ghcr.io/token?scope=repository:ronggur/status-ronggur-my-id:pull" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -sI -H "Authorization: Bearer $T" \
  -H 'Accept: application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.index.v1+json' \
  "https://ghcr.io/v2/ronggur/status-ronggur-my-id/manifests/$TAG" | grep -iE 'content-type|docker-content-digest'
```

## Deploy (dua region)

`--location` menerima daftar dipisah koma, jadi satu perintah membuat satu
placement per region. `--min=2` berarti dua instance per region, supaya
rollout tidak memutus layanan (rollout menggantikan satu instance sekali jalan).

```bash
datumctl compute deploy status-ronggur-my-id \
  --image=ghcr.io/ronggur/status-ronggur-my-id@sha256:<DIGEST> \
  --location=us-central-1,us-east-1 \
  --min=2 \
  --network=ronggur-my-id \
  --http-port=8080 \
  --instance-type=datumcloud/d1-standard-2 \
  --runtime-class=general-purpose \
  --yes \
  --project=personal-project-86e0525b
```

Cek `datumctl compute workloads --project=personal-project-86e0525b` dan
`datumctl compute instances --workload=status-ronggur-my-id -o wide --project=personal-project-86e0525b`.
Perintah ini menulis `workload.yaml` di direktori kerja (sudah di-gitignore).

Untuk uji satu region dulu, ganti jadi `--location=us-central-1 --min=1`.

## Subdomain status.ronggur.my.id

Zone `ronggur-my-id-38gvr7` sudah di Datum DNS dan domain `ronggur.my.id`
sudah Verified, jadi subdomain tidak perlu verifikasi baru.

1. Hostname ke HTTPProxy (server-side apply, field manager sendiri):

   ```bash
   datumctl apply --server-side --field-manager=ronggur \
     -f datum/status-hostnames.yaml --project=personal-project-86e0525b
   ```

2. Datum membuat ALIAS `status` sendiri (DNSRecordSet berlabel
   `dns.datumapis.com/managed=true`), seperti record `www` situs utama. Pastikan
   belum ada A atau CNAME lain bernama `status` di zone, karena itu menahan
   record otomatis:

   ```bash
   datumctl get dnsrecordsets --project=personal-project-86e0525b
   ```

3. Tunggu `Programmed=True` dan `CertificatesReady=True` (sekitar satu menit):

   ```bash
   datumctl get httpproxy status-ronggur-my-id --project=personal-project-86e0525b \
     -o jsonpath='{range .status.hostnameStatuses[*]}{.hostname}{": "}{range .conditions[*]}{.type}={.reason} {end}{"\n"}{end}'
   ```

## Verifikasi

```bash
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' http://status.ronggur.my.id/
# 301 https://status.ronggur.my.id/
curl -sS https://status.ronggur.my.id/healthz
# ok
curl -sS https://status.ronggur.my.id/api/status | python3 -m json.tool | head -40
```

Buka `https://status.ronggur.my.id/`. Status `degraded` pada probe `dns`
sebelum langkah subdomain selesai itu wajar.

## Yang perlu dicatat (ini bagian dari uji)

| Pertanyaan | Dilihat dari |
|---|---|
| Apakah ada outbound internet? | probe `outbound`, `http`, `tls`, `dns` di page |
| Apakah region instance bisa diketahui? | "Region" di kotak instance. Isinya "tidak diketahui" artinya hostname tidak memuat region |
| Dua region, satu hostname: ke mana trafik pergi? | muat ulang berkali-kali dan dari jaringan berbeda, bandingkan "Nama instance" dan "Alamat" |
| Berapa downtime rollout? | `while true; do curl -s -o /dev/null -w '%{http_code}\n' https://status.ronggur.my.id/healthz; sleep 0.5; done` lalu `datumctl compute restart status-ronggur-my-id` |
| Apakah `destroy` meninggalkan sisa? | setelah `destroy`, cek `datumctl get httpproxy` dan `datumctl get dnsrecordsets` |

Tulis hasilnya di `bug-log.md` (template di `datum-test-ideas.md`).

## Update

Ubah kode, push, jalankan **publish-status-image**, ambil digest baru, ulang
perintah deploy dengan digest itu (meng-update di tempat, hostname tetap).

Image terbaru (`status-e19bb77`, belum dideploy):
`ghcr.io/ronggur/status-ronggur-my-id@sha256:81775cc62c0d849fb60b70a0b730975ff64eb135df42232de74ad5e3a52685e4`

## Cabut

```bash
datumctl compute destroy status-ronggur-my-id --project=personal-project-86e0525b
```

`destroy` ikut menghapus HTTPProxy-nya, jadi record `status` otomatis ikut
hilang. Hostname datumproxy baru akan berbeda kalau deploy ulang, dan
`datum/status-hostnames.yaml` harus di-apply lagi.
