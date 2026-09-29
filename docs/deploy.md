# Deploy

Image Docker biasa, kelas `general-purpose`, project `personal-project-86e0525b`.
Server-nya `server/`. Network `ronggur-my-id` sudah ada.

## Image

```bash
docker login ghcr.io

docker buildx build \
  --platform linux/amd64 \
  --provenance=false --sbom=false \
  -f Dockerfile.unikraft \
  -t ghcr.io/ronggur/ronggur-my-id:container \
  --output type=image,push=true,oci-mediatypes=false .

docker buildx imagetools inspect ghcr.io/ronggur/ronggur-my-id:container \
  --format '{{.Manifest.Digest}}'
```

`--provenance=false`, `--sbom=false`, dan `oci-mediatypes=false` supaya yang
ter-push manifest Docker v2, bukan OCI index. Package harus publik.

Actions → **publish-image** melakukan build yang sama. Tag default
`rootfs-<sha>`.

Yang sedang jalan:
`ghcr.io/ronggur/ronggur-my-id@sha256:12e541067dbdc770dfaf2513c983ef9c162bc7294c196e8160215de71b3bac6c`

## Deploy

`--project` di belakang.

```bash
datumctl compute deploy ronggur-my-id \
  --image=ghcr.io/ronggur/ronggur-my-id@sha256:12e541067dbdc770dfaf2513c983ef9c162bc7294c196e8160215de71b3bac6c \
  --location=us-central-1 \
  --network=ronggur-my-id \
  --http-port=8080 \
  --instance-type=datumcloud/d1-standard-2 \
  --runtime-class=general-purpose \
  --yes \
  --project=personal-project-86e0525b
```

URL: https://avenue-shark-tjrc6.datumproxy.net (A dan AAAA). `http://`
di-redirect 301 ke https oleh server (`X-Forwarded-Proto`), plus HSTS.

Deploy ulang ke workload yang sama meng-update di tempat: HTTPProxy dan
`hostnames`-nya tetap. Perintah ini menulis `workload.yaml` di direktori kerja.
Jangan di-commit.

```bash
datumctl compute instances --workload=ronggur-my-id --project=personal-project-86e0525b
datumctl compute destroy ronggur-my-id --project=personal-project-86e0525b
```

Kalau network-nya hilang, buat sekali dari `datum/network.yaml`:

```bash
datumctl apply -f datum/network.yaml --project=personal-project-86e0525b
```

Tanpa `--network=ronggur-my-id`, workload menempel ke `default`, yang di
project ini IPv4 dan tidak pernah dapat alamat.

## Domain ronggur.my.id

`ronggur.my.id` dan `www.ronggur.my.id` dilayani workload ini sejak
2026-09-29. Sebelumnya keduanya A record ke `45.77.171.60`. Zone-nya
`ronggur-my-id-38gvr7` di Datum DNS, dan Domain `ronggur.my.id` sudah Verified.

| File | Isi |
|---|---|
| `datum/httpproxy-hostnames.yaml` | `spec.hostnames` untuk HTTPProxy buatan `compute deploy` |
| `datum/dns.yaml` | ALIAS `@` ke hostname datumproxy |

Record `www` **tidak** ada di repo. Begitu `www.ronggur.my.id` masuk
`hostnames`, Datum membuat ALIAS-nya sendiri (DNSRecordSet berlabel
`dns.datumapis.com/managed=true`, dimiliki Gateway `ronggur-my-id`). Untuk apex
Datum tidak membuatkan apa-apa, karena itu ada `datum/dns.yaml`.

### Memasang (atau memasang ulang setelah destroy + deploy)

HTTPProxy baru tidak punya `hostnames`, dan hostname datumproxy-nya bisa
berganti. Ambil dulu hostname-nya, lalu samakan `content` di `datum/dns.yaml`:

```bash
datumctl get httpproxy ronggur-my-id --project=personal-project-86e0525b \
  -o jsonpath='{.status.canonicalHostname}'
```

1. Hostname ke HTTPProxy. Server-side apply dengan field manager sendiri,
   supaya `spec.rules` milik controller compute tidak tersentuh:

   ```bash
   datumctl apply --server-side --field-manager=ronggur \
     -f datum/httpproxy-hostnames.yaml --project=personal-project-86e0525b
   ```

2. Singkirkan record lain di `@` dan `www`. A atau CNAME yang sudah ada di nama
   itu membuat record otomatis `www` tertahan (`DNSRecordProgrammed=Pending`)
   dan sertifikatnya tidak terbit.

   ```bash
   datumctl get dnsrecordsets --project=personal-project-86e0525b
   datumctl get dnsrecordsets <nama> -o yaml --project=personal-project-86e0525b > backup.yaml
   datumctl delete dnsrecordsets <nama> --project=personal-project-86e0525b
   ```

   Begitu recordset lama terhapus, apex tidak punya record sampai langkah 3.
   Jalankan langkah 3 langsung sesudahnya.

3. ALIAS apex:

   ```bash
   datumctl apply -f datum/dns.yaml --project=personal-project-86e0525b
   ```

4. Tunggu `Programmed=True` dan `CertificatesReady=True`. Dari record siap
   sampai sertifikat Let's Encrypt terbit butuh sekitar satu menit.

   ```bash
   datumctl get httpproxy ronggur-my-id --project=personal-project-86e0525b \
     -o jsonpath='{range .status.hostnameStatuses[*]}{.hostname}{": "}{range .conditions[*]}{.type}={.reason} {end}{"\n"}{end}'
   ```

### Verifikasi

`--connect-to` menembak IP Datum langsung, jadi bisa dipakai sebelum cache DNS
publik berganti:

```bash
dig +short A ronggur.my.id @ns1.datumdomains.net     # 67.14.164.1, 67.14.165.1
dig +short A ronggur.my.id @1.1.1.1
curl -sS -o /dev/null -w '%{http_code}\n' \
  --connect-to ronggur.my.id:443:67.14.164.1:443 https://ronggur.my.id/
echo | openssl s_client -connect 67.14.164.1:443 -servername ronggur.my.id 2>/dev/null \
  | openssl x509 -noout -subject -issuer
```

IPv6 (`2607:ed40:10::1`, `2607:ed40:20::1`) timeout dari laptop yang IPv6-nya
tidak sampai internet. Itu masalah laptopnya, bukan bukti proxy mati.

TTL A record lama 14400 detik. Resolver yang sudah menyimpannya masih
mengarah ke server lama sampai 4 jam, jadi server lama jangan dimatikan
sebelum itu.

### Kembali ke server lama

Kosongkan `hostnames` (record `www` buatan Datum ikut terhapus), hapus ALIAS
apex, lalu buat lagi A record lama. HTTPProxy-nya jangan dihapus: hostname
datumproxy-nya tidak kembali sama (lihat compute.md §4d).

```bash
datumctl apply --server-side --field-manager=ronggur --project=personal-project-86e0525b -f - <<'EOF'
apiVersion: networking.datumapis.com/v1alpha
kind: HTTPProxy
metadata: {name: ronggur-my-id, namespace: default}
spec: {hostnames: []}
EOF
datumctl delete dnsrecordsets ronggur-my-id-38gvr7-apex --project=personal-project-86e0525b
datumctl apply --project=personal-project-86e0525b -f - <<'EOF'
apiVersion: dns.networking.miloapis.com/v1alpha1
kind: DNSRecordSet
metadata: {name: ronggur-my-id-38gvr7-a, namespace: default}
spec:
  dnsZoneRef: {name: ronggur-my-id-38gvr7}
  recordType: A
  records:
  - {name: '@', ttl: 14400, a: {content: 45.77.171.60}}
  - {name: www, ttl: 14400, a: {content: 45.77.171.60}}
EOF
```

## Lokal

```bash
cd server && PORT=8080 SITE_ROOT=.. go run .
```
