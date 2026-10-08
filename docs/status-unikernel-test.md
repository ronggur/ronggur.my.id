# Uji status page: unikernel vs general-purpose

Tanggal: 2026-10-08. Pertanyaan: apakah halaman status (`status/`, satu binary
Go static) bisa jalan di runtime **unikernel** Datum Compute, dan apa bedanya
dengan runtime **general-purpose**? Acuan: walkthrough
`datum-cloud/twins-in-the-loop`, branch `feat/datum-compute-unikernel`,
`docs/datum-compute-walkthrough.md`.

## Setup

Dua workload uji, kode dan network (`ronggur-my-id`) sama, satu region
(`us-central-1`, `--min=1`), project `personal-project-86e0525b`:

| Workload | Runtime | Image | Hostname |
|---|---|---|---|
| `status-test-gp` | `general-purpose` | `docker buildx` (manifest docker v2) | `status-gp.ronggur.my.id` |
| `status-test-uk` | `unikernel` | `datumctl compute build` | `status-uk.ronggur.my.id` |

- Image di `ghcr.io/ronggur/status-ronggur-my-id`, tag `test-gp*` dan `test-uk*`
  (paket sudah publik). Digest terakhir: gp `sha256:ca578a6d...fb5d0`
  (`test-gp3`), uk `sha256:0929e4b6...412ec` (`test-uk3`).
- Hostname dibuat lewat `datum/status-test-hostnames.yaml` (dua HTTPProxy,
  server-side apply, field manager `ronggur`). Datum membuat ALIAS DNS-nya
  sendiri.
- `PROBE_DNS_HOSTS` diisi per workload (`ronggur.my.id,status-<gp|uk>.ronggur.my.id`)
  supaya kartu DNS menampilkan host yang benar.
- Workload `status-ronggur-my-id` (produksi `status.ronggur.my.id`) **belum
  pernah dideploy**; situs utama `ronggur-my-id` tidak disentuh.

Prasyarat di macOS: Docker Desktop harus jalan (tanpa itu `datumctl compute
build` gagal dengan `could not connect to BuildKit`). Dry run:
`datumctl compute build --analyze --fix .` menghasilkan "No compatibility issues
found" dan Dockerfile tidak berubah.

## Temuan

1. **Build unikernel bersih**: image sekitar 6 MB, tanpa isu kompatibilitas.
2. **Platform hanya punya IPv6 egress.** IPv4 literal (`1.1.1.1:443`) selalu
   `network is unreachable`, di dua runtime dan di instance contoh lain
   (`zac-test.joecompany.com`). Egress IPv6 ke Cloudflare dan Google jalan
   (HTTP 204 dari Google 2 sampai 36 ms).
3. **Unikernel: tidak ada loopback.** `127.0.0.1` dan `[::1]` unreachable bahkan
   setelah jaringan naik. Probe `self` lama (HTTP ke `127.0.0.1`) membuat status
   halaman selalu `down` walau server sehat dan melayani trafik dari edge.
4. **Unikernel: tidak ada `/etc/resolv.conf`.** Resolver Go default ke `[::1]:53`
   dan timeout. DNS lewat server IPv6 eksplisit jalan. general-purpose mendapat
   resolver DNS64 dari platform (`2001:4860:4860::6464`).
5. **Jendela awal setelah boot.** Ronde probe pertama (detik ke-0) gagal di
   kedua runtime: di unikernel `network is unreachable`, di general-purpose
   timeout. Dari ronde kedua semuanya normal. Di unikernel rute pernah baru
   muncul beberapa menit setelah boot (sebelum ada penantian rute); di deploy
   terakhir ronde pertama sudah sukses. Belum cukup sampel untuk menyimpulkan
   apakah selalu siap.
6. **Timer unikernel lambat.** Jam dinding benar, tapi interval probe 30 detik
   muncul tiap 87 sampai 167 detik, dan uptime hanya naik sekitar 0,24 detik per
   detik. Penyebab belum diketahui; tidak ada perintah `logs` untuk unikernel.
7. **Dari dalam Datum, edge Datum sendiri tidak terjangkau.** TCP ke
   `[2607:ed40:10::1]:443` dan `[2607:ed40:20::1]:443/80` (alamat
   `ronggur.my.id`) timeout, sementara Google dan Cloudflare normal dan dari luar
   Datum terjangkau. Gejalanya di level jaringan, bukan TLS/HTTP/aplikasi. Dugaan
   (belum terbukti): hairpin, prefix `2607:ed40::/32` dirutekan internal tanpa NAT
   untuk alamat sumber `fd20::`. Perlu dilaporkan ke tim Datum. Akibatnya probe
   `http` dan `tls` ke situs sendiri selalu merah selama halaman berjalan di
   dalam Datum.

## Perubahan kode (`status/`)

Dikerjakan dengan TDD (tes dulu, lalu kode; `go vet` dan `go test -race` lulus):

- `self` dipanggil **in-process** (`selfProbe`), tanpa soket.
- Resolver fallback `DNS_FALLBACK` (`[2001:4860:4860::6464]:53`) hanya bila
  `/etc/resolv.conf` tidak memuat nameserver (`hasNameserver`, `newResolver`);
  dipasang sebagai `net.DefaultResolver` agar probe HTTP dan TLS ikut memakainya.
- Menunggu rute siap sebelum ronde pertama, maksimal 15 percobaan sedetik
  (`waitForRoute`, `routeAvailable`); jika tidak siap, tetap jalan dan melaporkan
  kegagalannya.
- Default `PROBE_OUTBOUND_ADDR` jadi IPv6 (`[2606:4700:4700::1111]:443`).
- Probe baru `google` (`GET https://www.google.com/generate_204`,
  `PROBE_GOOGLE_URL`), tidak critical.
- Seluruh teks di `status/` (halaman, komentar, tes, README) diterjemahkan ke
  bahasa Inggris. README paragraf "tidak ada outbound internet" dikoreksi.

## Hasil akhir (deploy `test-gp3` dan `test-uk3`)

| Probe | gp | uk |
|---|---|---|
| `self` | ok | ok |
| `dns` | ok | ok |
| `google` | ok | ok |
| `outbound` (IPv6) | ok | ok |
| `http`, `tls` ke `ronggur.my.id` | timeout (temuan 7) | timeout (temuan 7) |
| Status halaman | `degraded` | `degraded` (sebelumnya `down`) |

## Masih berjalan

Dua workload, dua HTTPProxy, dan dua record DNS uji **sengaja dibiarkan nyala**
atas permintaan. Membersihkan:

```bash
P=--project=personal-project-86e0525b
datumctl compute destroy status-test-gp --yes $P
datumctl compute destroy status-test-uk --yes $P
git rm datum/status-test-hostnames.yaml
```

`destroy` ikut menghapus HTTPProxy dan record `status-gp`/`status-uk`. Tag
`test-*` di GHCR dihapus lewat Package settings.

## Belum dikerjakan

- Lapor ke tim Datum: loopback, resolv.conf, timer lambat, dan edge yang tidak
  terjangkau dari dalam platform.
- Probe `http` ke alamat internal workload utama
  (`http://[fd20:0:8::1:0:0]:8080/healthz`) sebagai pengganti pengecekan edge.
- Deploy produksi `status-ronggur-my-id` (lihat `deploy-status.md`).
- Sampel lebih banyak untuk jendela rute saat boot di unikernel.
