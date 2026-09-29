# Catatan kerja: menyiapkan ronggur.my.id untuk Datum Compute

Tanggal: 2026-08-19. Sumber acuan: `compute-handbook` (README, getting-started,
integrating-your-app, reference/limits register, api-reference,
ci-cd-and-operations, troubleshooting). Kode `L<angka>` merujuk baris limits
register di `compute-handbook/reference.md`.

Panduan yang berlaku: [docs/deploy.md](docs/deploy.md).
File ini adalah catatan **apa yang dikerjakan, kenapa, dan apa hasil tesnya**.
Isinya ditulis berurutan waktu. Bagian lama yang menyebut `deploy/datum/`,
`deploy-manual.md`, skrip, kustomize, atau jalur unikernel menggambarkan
keadaan saat itu; semua itu sudah dihapus dari repo (2026-09-29).

**Keadaan per 2026-09-29:**

- Workload `ronggur-my-id` jalan di `us-central-1`, kelas `general-purpose`,
  image `ghcr.io/ronggur/ronggur-my-id@sha256:abd98747…`, network IPv6
  `ronggur-my-id`. Di-deploy dengan satu `datumctl compute deploy` (§7).
- `https://ronggur.my.id`, `https://www.ronggur.my.id`, dan
  `https://avenue-shark-tjrc6.datumproxy.net` menyajikan situs ini.
  Terbukti lewat IPv4; IPv6 belum teruji karena laptop tes tidak punya
  IPv6 ke internet (§8).
- Repo: `server/` (server Go), `Dockerfile.unikraft` (image yang di-push),
  `datum/` (network, hostname HTTPProxy, ALIAS apex), `docs/deploy.md`.
  Workflow `publish-image` hanya build + push image container.

## 1. Kesimpulan kelayakan

Enam pertanyaan qualify di handbook, dijawab untuk situs ini:

| # | Pertanyaan | Jawaban |
|---|---|---|
| 1 | HTTP satu port? | Ya — server statis, satu port |
| 2 | Tiap replica disposable? | Ya — tidak ada state sama sekali |
| 3 | Satu replica muat 1 vCPU / 2 GiB? | Ya — situs statis 1,1 MB |
| 4 | Config lewat env? | Ya — `PORT`, `SITE_ROOT`; tanpa Secret |
| 5 | Bisa di-rebuild jadi unikernel? | Ya — Dockerfile milik sendiri |
| 6 | Bisa dioperasikan dari luar? | Ya — ada `/healthz`, verifikasi lewat curl |

Enam "ya" → kasus yang memang muat hari ini, sekelas contoh milo-os.com di
handbook. Tidak ada komponen yang perlu ditinggal.

## 2. Keputusan desain

| Keputusan | Alasan |
|---|---|
| Server statis ditulis di Go, `CGO_ENABLED=0` | Rootfs `FROM scratch` tidak punya loader maupun shared library. Binary statis menghilangkan jebakan klasik handbook: satu `.so` lupa dicopy → unikernel mati beberapa detik setelah boot tanpa error, dan platform tidak punya log sama sekali (L6) |
| nginx **tidak** dipakai di unikernel | Perlu menyalin binary + musl loader + pcre2/zlib/openssl dan tetap rawan; nilainya tidak sebanding untuk menyajikan satu halaman statis |
| Registry: ghcr.io publik | Cell harus bisa pull tanpa credential per-workload (L9); `index.unikraft.io` menolak semua pull anonim, dan itu persis penyebab image milo-os.com stall saat boot. Registry tetap dibutuhkan meski build dilakukan dari laptop — cell menarik image dari sana, bukan dari mesinmu |
| Deploy tidak lewat CI | Keputusan pemilik repo (2026-08-19). Workflow yang tersisa hanya build+push opsional dan manual-dispatch |
| Digest pin lewat kustomize `images:` | Re-push tag mutable tidak me-roll apa pun (L52). Blok `images:` polos sudah cukup, tanpa fieldspec (L13) — diverifikasi di sini |
| Deploy selalu `-f`, tidak pernah bentuk flags | Bentuk flags membangun ulang seluruh spec dari flags dan menimpa `workload.yaml` di direktori kerja (L54) |
| Port 8080 | Bebas; edge yang menerminasi TLS, jadi 80 tidak memberi keuntungan |
| `minReplicas: 1` | Cukup untuk test. Tanpa surge, tiap rollout memutus layanan (L21, L43) — dinaikkan ke 2 kalau nanti dipakai sungguhan |
| `index.html` saja yang masuk image | Permintaan eksplisit; `flappy.html` tetap di repo untuk pemakaian lokal |
| Placement DFW | Satu-satunya kota yang terbukti live di verification run handbook. `personal-project-86e0525b` mengikat `dfw`, `iad`, `sjc` — ketiganya Available |

## 3. Yang ditambahkan ke repo

> **Sejarah.** Daftar di bawah adalah isi repo 2026-08-19 sampai 2026-09-22.
> Yang tersisa sekarang ada di "Keadaan per 2026-09-29" di atas.

> **Update 2026-09-22.** Dua file baru di `deploy/datum/base/`:
> `networkservice.yaml` dan `httpproxy.yaml` — pasangan objek yang menerbitkan
> URL HTTPS terkelola (§4c), sudah terdaftar di `base/kustomization.yaml`.
> `base/workload.yaml` dapat `runtime.class: general-purpose` dan
> `locationSelector`, dan tag di overlay production pindah dari `latest` ke
> `container` supaya artifact unikernel lama dan image container baru tidak
> saling menimpa di package ghcr yang sama. Catat juga: `/workload.yaml` di
> root itu **gitignored** — salinan siap-deploy, bukan sumber kebenaran.
>
> Skrip di `deploy/datum/scripts/` juga bertambah dan berubah:
> `build-container.sh` (docker build biasa untuk kelas `general-purpose`),
> `deploy-flags.sh` (jalur tanpa manifest, dengan `--http-port`), `url.sh`
> (baca hostname terkelola), `deploy.sh` ditulis ulang ke `datumctl apply`
> karena render sekarang membawa tiga objek, dan `verify.sh` ditulis ulang
> karena versi lamanya meng-curl `externalIP` yang tidak akan pernah ada.
> Semuanya menembak `--project personal-project-86e0525b` secara eksplisit.

```
Kraftfile                              spec v0.7, runtime base-compat:latest, cmd ["/server"]
Dockerfile.unikraft                    Go static build -> FROM scratch (+ guard ldd)
project.yaml                           terisi (tadinya 0 byte)
compute.md                             file ini
deploy-manual.md                       panduan manual 11 langkah + tabel error
.gitignore                             workload.yaml (hasil render), .DS_Store
.dockerignore                          + .github, workload.yaml, evidence dir
README.md                              section "Deploy ke Datum Compute"
deploy/datum/
  README.md                            runbook: status akun, 2 STOP gate, 11 langkah, operasi harian
  server/{main.go,go.mod}              server statis: PORT, SITE_ROOT, /healthz
  base/workload.yaml                   Workload + kustomization
  overlays/production/                 digest pin
  ingress/{gateway,endpointslice,httproute}.yaml
  scripts/{build-push,deploy,repoint-ingress,verify,serve-local,common}.sh
.github/workflows/publish-image.yml    manual-dispatch, dua mode: rootfs (tanpa secret) / unikernel
```

Yang **tidak** disentuh: `index.html`, `flappy.html`, `flappy-bird-assets/`,
`Dockerfile` (nginx), `docker-compose.yml`. Jalur pengembangan lokal tetap sama
persis seperti sebelumnya.

## 4. Hasil test

### Terbukti jalan

| Yang diuji | Hasil |
|---|---|
| `go vet`, `gofmt` | Bersih |
| Cross-compile `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` | ELF 64-bit x86-64, **statically linked**, 5,4 MB |
| Server lokal (macOS) | `/` 200 html · sprite PNG 200 · `wing.wav` → `audio/wav` · `favicon.ico` → `image/x-icon` · `/healthz` 200 · file hilang 404 · direktori 404 tanpa listing · path traversal biasa maupun ter-encode 404 · POST 405 · Range 206 · `Cache-Control` beda html vs asset |
| `docker build --platform linux/amd64 -f Dockerfile.unikraft` | Sukses ~85 detik (cross-compile lewat qemu), guard `ldd` lolos, image 6,2 MB |
| Menjalankan image hasil build (`docker run --platform linux/amd64 …`) | `serving /site on :8080` → `/` 200 · sprite 200 · wav 200 · favicon 200 · `/healthz` ok · `/flappy.html` 404 (memang sengaja tidak ikut) |
| `repoint-ingress.sh` dengan `datumctl` palsu | EndpointSlice benar; instance tanpa external IP dilewati |
| `verify.sh` saat instance tidak menjawab | Gagal keras, exit 1 |
| Render `overlays/production` (kustomize v5.6.0 bawaan kubectl) | Satu dokumen; blok `images:` dengan digest benar-benar menulis ulang `spec.template.spec.runtime.sandbox.containers[].image` — mengonfirmasi L13 |
| Semua YAML + semua skrip | Parse bersih (`YAML.load_file`, `bash -n`) |
| **Rantai registry ghcr.io, end to end** | Workflow mode `rootfs` build + push tanpa secret apa pun (pakai `GITHUB_TOKEN`); hasilnya `ghcr.io/ronggur/ronggur-my-id@sha256:abd98747…` bisa ditarik **anonim**: token endpoint memberi token, manifest 200, tags list 200, config blob 200 — `amd64/linux`, `cmd ["/server"]`, `8080/tcp`, label source menunjuk repo. 4 lapis, 2,7 MiB (2026-08-19) |

Dua perbaikan lahir dari test:

1. `FROM scratch` awalnya tanpa pin platform, dan image config-nya mengaku
   `arm64` padahal isinya binary x86_64 — `docker run --platform linux/amd64`
   langsung menolak. Sekarang kedua stage dipin, dan build manual
   didokumentasikan memakai `docker build --platform linux/amd64`.
2. Image scratch tanpa `CMD` bikin `docker run` (dan tombol Run di Docker
   Desktop) gagal dengan `no command specified`. Ditambahkan `EXPOSE 8080` dan
   `CMD ["/server"]` — murni untuk kenyamanan lokal: di Datum yang menjalankan
   proses adalah `cmd:` di `Kraftfile`, kraftlet tidak membaca config OCI ini.

### Blocked — dan ini penghalang nyata

`kraft pkg` tidak bisa mengambil runtime dasarnya:

```
$ kraft pkg --name ronggur-my-id:test --plat kraftcloud --arch x86_64 .
level=error msg="could not package: could not find runtime
  'index.unikraft.io/official/base-compat:latest' (kraftcloud/x86_64)"
```

Sama juga dengan `--plat qemu`, dan `kraft pkg pull` atas referensi yang sama
gagal menemukannya. Ini persis L51: `index.unikraft.io` menolak pull anonim dan
akses ke base image enterprise dikontrol. **Tanpa `kraft login` dengan akun yang
punya akses base, tidak ada image unikernel yang bisa dibuat**, jadi tidak ada
yang bisa dideploy. Rootfs-nya sendiri sudah terbukti benar — yang kurang hanya
runtime pembungkusnya.

> **Tidak berlaku lagi per 2026-09-22.** Kelas `general-purpose` menjalankan
> image container biasa, jadi tidak ada image unikernel yang perlu dibuat dan
> `base-compat` tidak perlu diambil sama sekali. Lihat [§4c](#4c-temuan-besar-2026-09-22-runtime-class-membuka-l51-url-terkelola-membuka-l8).

### Belum diuji sama sekali

Deploy ke Datum dan jembatan ingress (Gateway → EndpointSlice → HTTPRoute).
Mode `unikernel` di `publish-image.yml` juga belum pernah jalan — ia menunggu
kredensial `index.unikraft.io`.

## 4b. Temuan besar (2026-08-25): staging IPv6-only, bukan bug kita

Scan langsung ke source `compute` dan `infra` (`/Users/ronggur/Works/Datum/www/`)
menemukan penyebab sesungguhnya `EXTERNAL IP` kita selalu kosong:

- `networkInterfaces[].ipFamilies` default ke `[IPv6]` kalau tidak diisi
  (`compute/api/v1alpha/instance_types.go`, sudah masuk ke CRD yang terpasang)
  — dan **semua** sample resmi di repo `compute` (termasuk yang kita ikuti)
  juga tidak mengisinya.
- `externalIP` butuh request class `public-ipv4` lewat `addresses[]`, tapi
  staging **tidak punya IPClass IPv4 sama sekali** — dikonfirmasi tertulis
  eksplisit di `infra/apps/network-services-operator/platform-project/classes.yaml`:
  *"IPv6 is the only address family the platform hands out. There is
  deliberately no IPv4 class here."*
- Bonus bug: `networkPolicy.ingress[].from[].ipBlock.cidr: 0.0.0.0/0` yang
  kita (dan semua sample resmi) pakai **tidak mencocokkan trafik apa pun**
  di interface IPv6-only. Sudah diperbaiki di `base/workload.yaml` — ditambah
  `ipBlock: cidr: "::/0"`.

**Dikonfirmasi independen oleh tim Datum sendiri** (Scot Schuchert-Wells, Slack,
2026-08-25 03:31): workload uji coba `ipv6-hello` di staging DFW mencapai
`STATUS: Available` dengan hanya alamat internal (`fd20::1:0:0/96`), tanpa
external IP — persis pola yang kita temukan dari source code.

**Implikasi:** kriteria sukses deploy kita bukan lagi "dapat EXTERNAL IP" (itu
tidak akan pernah terjadi di staging), tapi **"STATUS jadi Available"**.
Koreksi sudah ditambahkan ke `compute-handbook` sebagai row **L57** baru di
`reference.md` (section Networking & ingress) dan catatan di `getting-started.md`
sebelum section "Expose it on your domain".

## 4c. Temuan besar (2026-09-22): runtime class membuka L51, URL terkelola membuka L8

Scan ulang `compute` dan `infra`, lalu diverifikasi langsung ke staging
(`website-w7zf79`, sejak itu dihapus — lihat §4e). Keadaan awal yang
ditemukan: workload `ronggur-my-id`
**sudah Available sejak 2026-08-25**, 1/1 ready, image `compute-hello`,
`INTERNAL IP fd20:0:1::1:0:0/96`, tanpa external IP — persis kriteria sukses
yang ditetapkan di §4b. Yang berubah adalah apa yang sekarang mungkin di
atasnya.

### Runtime class: image Docker biasa akhirnya bisa jalan

Field baru `spec.template.spec.runtime.class`. Dibaca langsung dari project
pada 2026-09-22:

```
NAME              DISPLAY NAME         ISOLATION          DEFAULT  AVAILABLE  AGE
general-purpose   General purpose      virtual-machine             True       10d
unikernel         Unikernel fast path  unikernel          true     True       11d
```

- `unikernel` adalah **default platform**, jadi setiap workload yang dibuat
  sebelum field ini ada berjalan di situ. Itu sebabnya hanya image hasil
  `kraft` yang pernah boot.
- `general-purpose` dilayani `kata-provider`: tiap instance dapat VM ringan
  dengan kernel sendiri. Kontrak yang dipublikasikan kelas ini: *"Runs standard
  Linux container images without modification, including images that need a
  full filesystem, dynamic linking, or credentials to pull."* Feature-nya
  mencakup `sandboxRuntime`, `imagePullSecrets`, `configMapVolumes`,
  `secretVolumes`, `envFrom`, `containerCapabilities`.
  `defaultSecurityContext`-nya distempel saat admission: capability set Docker
  biasa (termasuk `NET_BIND_SERVICE`), `allowPrivilegeEscalation: false`,
  seccomp `RuntimeDefault`.

**Dua blocker lama gugur sekaligus:**

- **L51 (akses enterprise `base-compat`) tidak lagi relevan.**
  `Dockerfile.unikraft` sebenarnya sudah menghasilkan image OCI biasa yang sah
  — `FROM scratch`, satu binary statis, `/site`, `CMD ["/server"]` — jadi
  `docker build -f Dockerfile.unikraft` saja cukup. Tanpa `kraft`, tanpa
  Kraftfile, tanpa `kraft login`.
- **L9 (package ghcr harus publik) tidak lagi wajib**, karena kelas ini
  melayani `imagePullSecrets`.

**Harganya:** startup beberapa detik (boot kernel + pull image), lebih mahal
per instance, dan `suspend`/`resume`/`snapshot` belum ditawarkan.

**Kelas itu immutable, dan tidak bisa ditambahkan ke workload yang sudah
jalan.** Dry-run server pada 2026-09-22 menolaknya apa adanya:

```
spec.template.spec.runtime.class: Forbidden: may only be set to "unikernel"
on an existing workload, which is the class it already runs in
```

Jadi pindah tier = `datumctl compute destroy ronggur-my-id` dulu, baru apply,
atau deploy dengan nama kedua supaya yang lama tetap melayani sampai yang baru
terbukti. Yang tidak bisa dibaca dari mana pun: apakah lokasi (`us-central-1`)
melayani kelas itu. Workload tetap dibuat, lalu tidak menempatkan apa-apa dan
melaporkan `RuntimeClassNotServed` di placement.

### URL HTTPS terkelola: jembatan ingress manual tidak diperlukan lagi

`datumctl compute deploy --http-port=8080` menerbitkan workload di URL HTTPS
milik Datum. Di belakangnya dua objek, keduanya bisa ditulis tangan
(`compute/internal/cmd/compute/url/resources.go`):

- `NetworkService` — memilih network interface workload **lewat label**, jadi
  instance yang muncul, hilang, dan pindah kota tidak perlu diedit.
- `HTTPProxy` — satu rule, satu backend yang menunjuk NetworkService itu.
  `spec.hostnames` menerima custom hostname, jadi `ronggur.my.id` bisa
  dipasang di sini.

Ini menggantikan seluruh rantai Gateway → EndpointSlice → HTTPRoute di §6
langkah 7, **berikut kewajiban menjalankan `repoint-ingress.sh` setiap deploy
(L8)** — yang justru satu-satunya alasan chore itu ada.

⚠️ **`--http-port` tidak bisa digabung dengan `-f`**: *"a manifest declares its
own ports, and declaring an HTTP service in a manifest is not supported yet"*.
Dari jalur manifest, dua objek itu ditulis sendiri. Dan `deploy` sama sekali
tidak punya flag `--env`, jadi jalur flag hanya setara kalau semua konfigurasi
sudah jadi default di dalam image (untuk kita kebetulan iya:
`env("PORT","8080")`, `env("SITE_ROOT","/site")`).

### IPv4: ada di API, belum ada di platform

`networkInterfaces[].addresses[].class: public-ipv4` sekarang ada di API, dan
**immutable saat create**. Tapi `infra/.../platform-project/classes.yaml` tidak
berubah sejak 2026-08-19 dan masih menyatakan tidak ada IPClass IPv4, dengan
`infra/network/ipam/ipv4.yaml` masih `status: TODO`. Karena setiap family yang
diminta harus bisa dipenuhi atau interface tidak pernah published, meminta IPv4
hari ini menghasilkan interface yang tidak pernah naik. **§4b masih berlaku
utuh.**

### Perubahan kecil yang sudah kena file ini

- `placements[].cityCodes` **deprecated**, ditulis ulang server-side jadi
  `locationSelector` pada label `topology.datum.net/city-code`. Ketahuan karena
  workload kita sendiri kembali sebagai generation 2 pada 2026-09-17 sudah
  termigrasi. `workload.yaml` sekarang menulis bentuk barunya.
- `--port` dihapus dari `deploy`, diganti `--http-port`. Flag baru lain:
  `--runtime-class`, `--network`, `--location-selector`, `--build`, `--no-http`.
- `datumctl compute destroy` sekarang ikut menurunkan URL workload.
- `datumctl compute build` sudah nyata di repo (Dockerfile → unikernel OCI,
  plus deteksi dan `--fix` untuk non-PIE entrypoint dan shared library hilang).
  Hanya relevan kalau tetap di kelas `unikernel`.
- UI menampilkan metrics, topology, logs, dan alasan kondisi instance yang
  spesifik (`ImageUnavailable`, dll) — berguna kalau rollout nyangkut.
- **Penempatan `--project` bukan bebas.** Untuk perintah plugin, global flag
  harus di **belakang**: `datumctl compute instances --workload=W --project=P`
  jalan, sementara `datumctl --project=P compute instances --workload=W` mati
  dengan `unknown flag: --workload` dan mencetak usage `datumctl` sendiri —
  plugin hanya menerima argumen setelah namanya. Untuk perintah bawaan
  (`get`, `apply`, `explain`) dua-duanya jalan.

### CLI terpasang ketinggalan — ini langkah nol

```
$ datumctl plugin list
compute  v0.8.0-dev.10   ...
$ datumctl compute deploy --help
--city --file --image --instance-type --min --port --yes
```

Tidak ada `--runtime-class`, `--http-port`, `--build`, `--network`. Repo
`compute` sudah di tag `v0.10.3` dan `--http-port` mendarat 2026-09-09; client
`datumctl` v0.18.2 sementara repo menyematkan v0.19.0. **Update `datumctl` dan
plugin compute sebelum apa pun di bawah ini.** Jalur `datumctl apply -f` tidak
terpengaruh — ia bicara langsung ke API server, dan itu yang dipakai untuk
memvalidasi `workload.yaml` baru (`--dry-run=server`: diterima).

## 4d. Verifikasi silang (2026-09-22): demo resmi Datum, dan image yang ternyata sudah ada

Tiga sumber dicek: repo `datumctl`, repo `compute-network-demo`, dan preview
docs Mintlify `datum-4926dda5-docs-compute-overview-cli`.

### `compute-network-demo` — deployment referensi resmi, baru kemarin

Repo `datum-labs/compute-network-demo` ("Global Mesh", commit pertama
2026-09-21) adalah demo produk yang **benar-benar dideploy di Datum**: satu
workload, tiga kota, satu private network. Isinya `deploy/00-network.yaml`,
`10-workload.yaml`, `20-ingress.yaml`, plus varian `unikernel/` dan `live/`.

Bentuk yang mereka pakai identik dengan yang sudah kita tulis — `runtime.class:
general-purpose`, `NetworkService` yang memilih interface lewat
`compute.datumapis.com/workload-name`, `HTTPProxy` dengan backend
`networkService`. Empat hal dari sana yang kita adopsi:

- **`securityContext` ditulis eksplisit** dengan `capabilities: drop: [ALL]`,
  `allowPrivilegeEscalation: false`, `seccompProfile: RuntimeDefault`. Kalau
  dikosongkan, kelas `general-purpose` justru **menambah** capability set Docker
  biasa (CHOWN, SETUID, SETGID, NET_BIND_SERVICE, dan lainnya). Server statis
  kita tidak butuh satu pun — port 8080 di atas 1024, jadi melepas
  NET_BIND_SERVICE tidak ada biayanya.
- **`trafficDistribution.strategy: Nearest`** ditulis walau itu default;
  dokumentasi field-nya sendiri bilang yang menuliskannya hari ini tetap jalan
  saat strategi lain ditambahkan.
- **`weight: 1` dan `matches` PathPrefix `/`** ditulis eksplisit di HTTPProxy.
- **`name: eth0` dan `ipFamilies: [IPv6]`** ditulis, bukan dibiarkan default —
  keduanya immutable, jadi pantas terlihat di file yang membuatnya.

Dua hal lain yang belum kita pakai tapi tercatat: demo memakai `Network`
sendiri (`ipFamilies: [IPv6]`, `ipam.mode: Auto`, `mtu: 1440` karena jaringannya
dibawa lewat enkapsulasi) alih-alih `default`, dan demo **tidak memasang
networkPolicy sama sekali** di interface-nya.

⚠️ **Hostname HTTPProxy tidak stabil terhadap delete-recreate.** Dari
`deploy/README.md`: "deleting and recreating the HTTPProxy gives you a **new
hostname**". Ini bertemu langsung dengan §4c: `datumctl compute destroy` ikut
menurunkan URL workload, jadi kalau tier ditukar setelah link dibagikan,
link-nya mati. Urutannya: tukar kelas **sebelum** ada yang memegang URL-nya.

### Image container-nya ternyata sudah ada di ghcr

`crane ls ghcr.io/ronggur/ronggur-my-id` mengembalikan dua tag, dan yang
pertama bukan unikernel:

| Tag | mediaType | Isi |
|---|---|---|
| `rootfs-130e6af` | `application/vnd.docker.distribution.manifest.v2+json` | image Docker biasa, linux/amd64, `Cmd ["/server"]`, port 8080, `WorkingDir /` |
| `unikraft-fb036aa` | `application/vnd.oci.image.index.v1+json` | index OCI, platform `kraftcloud/x86_64`, `CONFIG_EROFS_FS=y` |

Mode `rootfs` di `publish-image.yml` selama ini mem-build `Dockerfile.unikraft`
sebagai image Docker biasa — dan itu **persis** yang dibutuhkan kelas
`general-purpose`. Pull anonim ke tag itu dijawab HTTP 200, jadi package-nya
publik dan tidak perlu `imagePullSecrets`. Overlay production sekarang menyemat
digest-nya:

```
sha256:abd987474d45c336aeb2607e10fef0830556b6241158daf16d0d80b7981b4ba4
```

**Artinya langkah build di §6 tidak wajib untuk deploy pertama.** Yang tersisa
tinggal `destroy` + `apply`.

### Media type image itu syarat, bukan detail

Dari `docs/publishing.md` demo: attestation mengubah artifact yang di-push
menjadi **OCI image index**, sementara runtime Datum memilih
`application/vnd.docker.distribution.manifest.v2+json` yang polos. Karena itu
build mereka memakai:

```sh
docker buildx build \
  --platform linux/amd64 \
  --provenance=false --sbom=false \
  --output type=image,push=true,oci-mediatypes=false .
```

`oci-mediatypes` adalah opsi exporter, bukan flag build — itu sebabnya push
lewat `--output`, bukan `--push`. Dan linux/amd64 saja: push multi-platform
adalah manifest list menurut definisinya, masalah yang sama.

Tag `rootfs-130e6af` kita kebetulan sudah benar, karena builder default runner
melakukan hal yang tepat tanpa diminta. Itu kecelakaan yang beruntung, bukan
jaminan — `publish-image.yml` dan `build-container.sh` sekarang menyatakannya,
dan `build-container.sh` membaca media type kembali dari registry setelah push
lalu gagal kalau yang keluar bukan manifest v2.

### `datumctl compute build` memilih `Dockerfile.datum`, bukan `Dockerfile`

Kalau nanti kembali ke jalur unikernel: `build` memakai `Dockerfile.datum` dari
build context bila ada, baru jatuh ke `Dockerfile`. File kita bernama
`Dockerfile.unikraft`, jadi butuh `-f` atau ganti nama. Perintah lengkapnya,
dari demo:

```sh
datumctl compute build --analyze --push --output <REGISTRY>/<NS>/<NAME>:<TAG> .
```

Dua syarat Dockerfile untuk packager itu, keduanya sudah kita penuhi: stage
akhir harus `scratch` (rootfs ditahan di RAM guest), dan `WORKDIR` harus tetap
`/` — packager membungkus entrypoint dengan shell untuk meniru WORKDIR non-root,
dan image scratch tidak punya shell. Base distroless `:nonroot` gagal justru
karena menyetel `WORKDIR /home/nonroot`.

### Preview docs Mintlify tertinggal dari kode

Halaman `/datumctl/compute/deploying-workloads` masih mendokumentasikan
`--port` (yang sudah dihapus dan diganti `--http-port`) dan masih menyatakan
"Compute boots workloads as unikernels" — yaitu keadaan sebelum runtime class
ada. **Jangan dipakai sebagai acuan untuk runtime class atau URL terkelola**;
repo `compute` dan `compute-network-demo` yang benar.

### `datumctl`: tidak ada yang relevan

Commit sejak 2026-09-09 semuanya bump dependency keamanan, kecuali satu:
AI agent bawaan diganti plugin (`datumctl plugin install assistant`), 2026-09-12.
Tidak menyentuh compute.

## 4e. Keadaan baru (2026-09-22, sore): project dihapus, dan satu network yang rusak

`website-w7zf79` dihapus user. Semua di bawah ini dibaca langsung dari
`datumctl`, bukan dari repo.

### Yang tersisa

`datumctl ctx list` tinggal satu baris project: **`personal-project-86e0525b`**
di org `personal-org-86e0525b` — persis project yang dicatat di §5 sejak
2026-08-19, jadi `project.yaml` kembali benar dan tidak perlu diubah.

Project itu sudah entitled, dan kali ini quota-nya bisa dibaca:

| Resource | Limit | Terpakai |
|---|---|---|
| Workloads | 1000 | 0 |
| Instances | 10 | 0 |
| vCPUs | 40 | 0 |
| Memory | 80 GiB | 0 |

Tidak ada workload sama sekali. **Itu kabar baik untuk §4c:** runtime class
immutable hanya menyakitkan kalau workload-nya sudah ada. Sekarang papan tulis
kosong, jadi `class: general-purpose` cukup ditulis saat create — langkah
`destroy` di §6 tidak diperlukan lagi.

### CLI sudah tidak ketinggalan

"Langkah nol" di §4c selesai. `datumctl` sekarang **v0.19.0** (dari v0.18.2),
dan plugin compute **v0.8.0 dari index `datum`, trust `official`** — sebelumnya
`v0.8.0-dev.10` yang terpasang `(direct)` sebagai `third-party`. `deploy --help`
sekarang menampilkan `--runtime-class`, `--http-port`, `--build`, `--network`,
`--location`, `--location-selector`, `--city`, `--min`; `--port` sudah hilang
sepenuhnya. Jadi jalur B (`deploy-flags.sh`) sekarang benar-benar bisa
dijalankan.

Server juga berganti: `milo.0.0.0-main-20260919` → `milo.0.33.0`.

### Runtime class ada di sini juga

```
NAME              DISPLAY NAME          ISOLATION         DEFAULT  AVAILABLE  AGE
general-purpose   General purpose       virtual-machine            True       5d1h
unikernel         Unikernel fast path   unikernel         true     True       5d1h
```

Sama seperti di project lama, hanya lebih muda (5 hari vs 10–11 hari), yang
menegaskan ini control plane yang berbeda. Kesimpulan §4c tidak berubah:
`unikernel` tetap default, `general-purpose` harus diminta eksplisit.

### 16 lokasi di platform, tapi compute hanya di 3

`datumctl get locations` mengembalikan **16 lokasi, semuanya Ready**, dan
`datumctl get locationbindings` menandai keenam belasnya `AVAILABLE=True`:

```
ae-north-1 DXB   au-east-1 SYD   br-east-1 GRU   ca-east-1 TOR
cl-central-1 SCL de-central-1 FRA gb-south-1 LHR in-west-1 BOM
jp-east-1 TYO    nl-west-1 AMS   sg-central-1 SIN us-central-1 DFW
us-east-1 IAD    us-east-2 LGA   us-west-1 SJC   za-central-1 JNB
```

**Kedua daftar itu bukan daftar yang menentukan.** Admission Workload hanya
menerima tiga:

```
spec.placements[0].locations[0].name: Unsupported value: "DFW":
  supported values: "us-central-1", "us-east-1", "us-west-1"

spec.placements[0].locationSelector: Invalid value:
  "topology.datum.net/city-code=FRA": matches none of the locations where
  compute is available (us-central-1, us-east-1, us-west-1)
```

Jadi compute dilayani di **DFW (`us-central-1`), IAD (`us-east-1`), dan SJC
(`us-west-1`)** — persis tiga locationbinding yang tercatat untuk project ini
pada 2026-08-19 di §5. Lokasi lain punya binding, tapi bukan untuk compute.

Kabar baiknya: kedua bentuk gagal **dengan keras** di admission, bukan diam-diam
tidak menempatkan apa-apa. Selector yang tidak cocok pun ditolak, bukan
diterima lalu kosong.

`us-central-1`/DFW ada di daftar itu, jadi `locationSelector` di
`workload.yaml` tetap valid tanpa diubah. Placement kedua tetap mungkin — tapi
pilihannya IAD atau SJC, bukan 15 kota lain. `NetworkService` dengan
`strategy: Nearest` (§4d) memang dirancang untuk itu. Batasnya quota:
10 instance.

### ⚠️ Network `default` rusak, dan harus diperbaiki sebelum deploy apa pun

```
NAME                      IPV6PREFIX      READY   REASON
datum-demo-net-89283b0b   fd20:0:4::/48   True    Ready
default                                   False   IPv6Required
```

`spec.ipFamilies` network `default` berisi `[IPv4]`, dan server mengatakannya
terang-terangan:

> The platform addresses workloads over IPv6 and this network does not carry
> it, so nothing placed on this network can be given an address. Add IPv6 to
> `spec.ipFamilies`.

Ini kemungkinan besar sisa auto-create dari `datumctl compute deploy` yang
lama, sebelum platform mengunci IPv6-only (§4b). Akibatnya nyata: apa pun yang
menempel ke network itu **tidak dapat alamat sama sekali** — bukan "tidak dapat
external IP" seperti §4b, tapi tidak dapat alamat apa pun.

> **Keputusan di bawah ini dibatalkan beberapa jam kemudian — lihat §4g.**
> Bukti dari workload yang benar-benar jalan di project ini menunjukkan tooling
> Datum sendiri membuat network baru dan tidak pernah menyentuh `default`, dan
> bahwa `default` belum pernah memegang prefix sama sekali. `network.yaml`
> sekarang membuat network `ronggur-my-id`, bukan menambal `default`.

Diperbaiki lewat file baru `deploy/datum/base/network.yaml`, yang menambal
network `default` itu sendiri alih-alih membuat network kedua. Alasannya:
network sebuah workload **immutable saat create**, dan jalur flag
(`deploy --http-port`) menempelkan workload baru ke network bernama `default`
kecuali diberi `--network`. Menambal yang ini membuat **kedua** jalur jalan;
network privat hanya akan menyelamatkan jalur manifest.

`datumctl apply --dry-run=server` menerima tambalannya (`network/default
configured`), jadi `ipFamilies` memang mutable selama belum ada alokasi di
bawahnya — dan sekarang belum ada.

MTU disetel 1440, bukan 1460 yang sekarang dibawa objek itu. Dua sumber
independen di environment ini menyebut 1440: `compute-network-demo` menulisnya
dengan alasan "the network is carried over an encapsulation", dan satu-satunya
network yang Ready di project ini (`datum-demo-net-89283b0b`) juga 1440. Angka
1460 datang dari auto-create yang salah soal address family, jadi ia bukan
bukti apa pun.

## 4f. Konvensi kecil: semua flag ditulis `--flag=value`

`--image=x` dan `--image x` identik untuk hampir semua flag, jadi ini nyaris
soal gaya. Nyaris. `--build` punya nilai default opsional — help menampilkannya
sebagai `string[="."]` — dan pflag hanya menerima nilai untuk flag semacam itu
lewat `=`. Diuji langsung pada 2026-09-22, dengan `--http-port` + `--no-http`
sebagai pengaman supaya perintahnya ditolak di validasi paling awal dan tidak
membangun apa pun:

```
$ datumctl compute deploy probe --build . --http-port=8080 --no-http …
Error: accepts at most 1 arg(s), received 2

$ datumctl compute deploy probe --build=. --http-port=8080 --no-http …
Error: --http-port and --no-http cannot be combined
```

Yang pertama membaca `.` sebagai argumen posisi kedua. Tanpa nama workload di
depannya, `.` akan diam-diam menjadi **nama workload** — kesalahan yang baru
kelihatan setelah objeknya terbentuk. Hal yang sama berlaku untuk flag boolean
bernilai eksplisit (`--yes=false`, bukan `--yes false`) dan nilai yang diawali
`-`.

Karena itu semua perintah `datumctl` di repo ini — skrip, README, dan
deploy-manual.md — ditulis dengan `=`, sama seperti seluruh contoh resmi
datumctl. Satu kebiasaan yang tidak pernah salah lebih murah daripada mengingat
mana pengecualiannya.

Sekalian: `deploy-flags.sh` sekarang memakai `--location=us-central-1`, bukan
`--city=DFW`. Keduanya sampai ke tempat yang sama hari ini, tapi `--location`
divalidasi terhadap enum di admission — nilai salah ditolak sambil menyebut
daftar yang didukung — sementara `--city` lewat selector. Keduanya tetap gagal
dengan keras (§4e), tapi yang pertama memberi daftarnya sekaligus.

## 4g. Dibandingkan dengan workload yang benar-benar jalan (`datum-demo-a7c517fd`)

Di project ini ada satu workload yang **Available 1/1** sejak 2 jam sebelum
catatan ini ditulis, dibuat oleh tooling Datum sendiri
(`app.kubernetes.io/managed-by: compute-portal-demo`). Spec-nya adalah
pembanding terbaik yang tersedia, karena ia bukan contoh di repo melainkan
sesuatu yang benar-benar berjalan di control plane yang sama.

| | `datum-demo-a7c517fd` | `ronggur-my-id` (kita) |
|---|---|---|
| `runtime.class` | `unikernel` | `general-purpose` |
| image | `index.docker.io/scotwells/global-mesh-uk@sha256:…` | `ghcr.io/ronggur/ronggur-my-id@sha256:…` |
| network | `datum-demo-net-a7c517fd` (miliknya sendiri) | `ronggur-my-id` (miliknya sendiri) |
| `ipFamilies` | `[IPv6]` | `[IPv6]` |
| `name` interface | `eth0` | `eth0` |
| `reclaimPolicy` | `Delete` | `Delete` (default) |
| `networkPolicy` | **tidak ada** | ingress 8080 dari `::/0` + `0.0.0.0/0` |
| `securityContext` | `capabilities.drop: [ALL]` | + `allowPrivilegeEscalation: false`, seccomp `RuntimeDefault` |
| nama placement | `dfw` | `dfw` (diselaraskan) |
| URL | `—` | HTTPProxy + NetworkService |

### Yang diubah karena pembandingan ini

**Network: dari menambal `default` jadi punya sendiri.** Ini membalik keputusan
di §4e, dan alasannya bukti, bukan selera:

```
NAME                      MANAGED-BY            MTU    FAMILY  READY
datum-demo-net-89283b0b   compute-portal-demo   1440   IPv6    True
datum-demo-net-a7c517fd   compute-portal-demo   1440   IPv6    True
datum-demo-net-ea9ab6be   compute-portal-demo   1440   IPv6    True
default                   -                     1460   IPv4    False
```

Tooling Datum sendiri **tidak memperbaiki `default`** — ia membuat network
IPv6 baru tiap demo, dan ketiganya Ready. Sementara `default` punya
`status.ipam: null` dan hanya membawa kondisi `IPv6Required`; kondisi
`IPAMAllocated` tidak pernah muncul di sana, jadi ia belum pernah memegang
prefix sama sekali. Menambal `ipFamilies`-nya memang lolos `--dry-run=server`,
tapi tidak ada apa pun yang menunjukkan controller akan mengalokasikan prefix
setelahnya. Satu jalur belum teruji, satu lagi terbukti tiga kali di project
yang sama — jadi `base/network.yaml` sekarang membuat network `ronggur-my-id`.

Harganya satu flag: jalur flag menempel ke network bernama `default` kecuali
diberi tahu, jadi `deploy-flags.sh` meneruskan `--network=ronggur-my-id`.
Jalur manifest tidak butuh apa-apa, karena workload-nya menyebut network itu
langsung.

Sekalian: MTU 1440 yang di §4e masih berupa penalaran sekarang punya sumber
ketiga — ketiga network buatan `compute-portal-demo` memakainya.

**Nama placement `us` → `dfw`**, mengikuti workload itu.

### Yang sengaja tetap berbeda

- **`class: general-purpose`.** Demo itu `unikernel` karena image-nya memang
  artifact unikernel. Image kita image container biasa (§4d), jadi kelasnya
  harus general-purpose. Bahwa demo itu Available membuktikan kelas `unikernel`
  dilayani di `us-central-1`; ia tidak mengatakan apa pun tentang
  general-purpose di lokasi itu — itu baru ketahuan saat deploy, lewat
  `RuntimeClassNotServed` kalau gagal.
- **`securityContext` kita lebih lengkap.** Demo hanya menulis
  `capabilities.drop: [ALL]`, kemungkinan karena kelas `unikernel` tidak
  melayani `allowPrivilegeEscalation` dan `seccompProfile`. Kelas
  `general-purpose` melayani keduanya dan menulis keduanya sebagai default
  kelas, jadi menyatakannya membuat objek yang tersimpan menggambarkan apa yang
  benar-benar berjalan.
- **`networkPolicy` kita pertahankan** walau demo tidak punya. Ia hanya
  mengizinkan 8080 dari `::/0`, jadi edge tetap bisa mencapainya, dan ia
  mendokumentasikan port yang dilayani di tempat yang sama dengan port itu
  dideklarasikan.

## 5. Keadaan akun

> **Update 2026-09-22, sore:** `website-w7zf79` **dihapus user**, dan dengan
> itu workload `ronggur-my-id` yang sudah Available 27 hari ikut hilang.
> `datumctl ctx list` sekarang hanya menyisakan **`personal-project-86e0525b`**
> di org `personal-org-86e0525b` — persis project yang dicatat di bawah, jadi
> `project.yaml` kembali benar. Keadaan lengkapnya ada di §4e.
>
> Catatan koreksi: sebelum penghapusan, `ctx list` sempat menampilkan
> `personal-org-16bed4ea`/`personal-project-16bed4ea`. ID itu tidak muncul lagi
> setelah refresh, dan server version juga berganti
> (`milo.0.0.0-main-2026-09-19` → `milo.0.33.0`), jadi pembacaan itu
> environment yang berbeda, bukan rename akun.

> **Update 2026-08-25 (sudah tidak berlaku — project ini dihapus 2026-09-22):**
> test aktif jalan di project baru **`website-w7zf79`**, di environment staging
> `cloud.datum.net`. Admission mengonfirmasi kota yang didukung project itu
> adalah **`DFW`** saja. Deploy pertama sempat membuat `Network default`
> otomatis — dan itu ternyata penting, lihat §4e: network yang dibuat otomatis
> begitu lahir dengan `ipFamilies: [IPv4]` dan tidak pernah bisa Ready.

### Keadaan akun sebelumnya (read-only, 2026-08-19)

- `datumctl` login sebagai `rhabibun@datum.net`, plugin compute terpasang.
- Organisasi `personal-org-86e0525b`.
- Project `ronggur-my-id` **sudah ada**, tapi **belum punya ServiceEntitlement**
  → kedua STOP gate masih di depan kalau memakai project itu.
- Project `personal-project-86e0525b` **sudah entitled**: locationbindings
  `dfw`, `iad`, `sjc` semuanya Available, tanpa workload dan tanpa network.
- `project.yaml` sudah diarahkan ke `personal-project-86e0525b`, jadi test
  pertama melewati kedua STOP gate. Catatan: file itu **tidak perlu di-apply** —
  project-nya sudah ada, dan apply hanya akan menimpa annotation
  `kubernetes.io/description` project pribadi.

## 6. Langkah berikutnya, berurutan

Perintah yang berlaku ada di [docs/deploy.md](docs/deploy.md). Bagian ini
catatan rencana 2026-09-22, sebelum folder `deploy/datum` dihapus.

> **Hasil.** Langkah 1–4 selesai 2026-09-28 lewat jalur flags, bukan skrip
> (§7). Langkah 5 selesai 2026-09-29 (§8). Langkah 6 belum.

Ditulis ulang 2026-09-22 sore setelah §4e. Dibanding versi sebelumnya, tiga
langkah hilang: update CLI (sudah), build image (sudah ada), dan `destroy`
workload lama (project-nya ikut terhapus). Yang bertambah satu: network.

**Prasyarat yang sudah terpenuhi, jadi tidak ada yang perlu dikerjakan untuknya:**
project entitled dengan quota kosong, plugin compute `v0.8.0` official dengan
semua flag baru, runtime class `general-purpose` Available, image
`rootfs-130e6af` publik dan digest-nya tersemat di overlay.

1. **Cek context-nya benar.** Semua skrip memakai
   `PROJECT=personal-project-86e0525b` secara eksplisit, jadi context aktif
   tidak menentukan — tapi pastikan `datumctl whoami` memang akun yang sama.

2. **Deploy.** `deploy/datum/scripts/deploy.sh` merender overlay,
   memvalidasinya dengan `--dry-run=server`, lalu apply. Empat objek, dalam
   urutan ini: `Network` (tambalan IPv6 untuk `default`, §4e), `Workload`,
   `NetworkService`, `HTTPProxy`.

   Tidak ada lagi langkah `destroy`: tidak ada workload yang menghalangi, jadi
   `class: general-purpose` masuk saat create.

3. **Tonton instance naik.** `datumctl compute instances --workload
   ronggur-my-id --project personal-project-86e0525b`. Kriteria sukses tetap
   **`STATUS: Available`** (§4b) — instance beralamat IPv6 internal saja, dan
   itu memang yang diharapkan.

   Kalau placement kosong dengan alasan `RuntimeClassNotServed`, berarti
   `us-central-1` tidak melayani `general-purpose`. Dua lokasi lain melayani
   compute — `us-east-1` (IAD) dan `us-west-1` (SJC), §4e — jadi coba salah
   satunya sebelum menyerah ke `unikernel`; tetap dengan workload baru, karena
   kelasnya immutable.

4. **Ambil URL-nya.** `deploy/datum/scripts/url.sh` membaca hostname terkelola
   dari status HTTPProxy. TLS butuh satu-dua menit setelah HTTPProxy pertama
   kali dibuat. Lalu `deploy/datum/scripts/verify.sh` untuk curl-nya.

   Jangan menghapus dan membuat ulang HTTPProxy setelah link dibagikan —
   hostname-nya tidak bertahan (§4d).

5. **Hostname sendiri.** Buka komentar `hostnames: [ronggur.my.id]` di
   `base/httpproxy.yaml` setelah hostname terkelola terbukti. Itu akan membuat
   resource `Domain` otomatis yang harus diverifikasi dulu; project ini belum
   punya `Domain` maupun `DNSZone`.

6. **Baru pikirkan ketahanan.** `minReplicas: 2` supaya rollout tidak
   mematikan satu-satunya instance (L21, L43), dan — ini baru mungkin sejak
   §4e — placement kedua di kota lain. `NetworkService` dengan
   `strategy: Nearest` sudah siap melayaninya dari satu URL yang sama. Quota
   membatasi di 10 instance.

Jalur tanpa manifest (`deploy-flags.sh`) sekarang juga bisa dijalankan, tapi
ingat ia membangun ulang spec dari flag: `securityContext`, `networkPolicy`,
dan env akan hilang (L54). Untuk situs ini env-nya kebetulan sudah jadi default
di dalam image, tapi `drop: [ALL]` tidak.

`deploy/datum/ingress/*` dan `repoint-ingress.sh` sudah tidak dipakai sejak
§4c. Hapus setelah URL terbukti jalan.

## 7. Tes jalur flags (2026-09-28)

Jalur B dijalankan hidup, bukan hanya dry-run. Perintah yang berlaku ada di
[docs/deploy.md](docs/deploy.md). Ringkasnya: `datum/network.yaml` di-apply
dulu, lalu

```sh
datumctl compute deploy ronggur-my-id \
  --image=ghcr.io/ronggur/ronggur-my-id@sha256:abd987474d45c336aeb2607e10fef0830556b6241158daf16d0d80b7981b4ba4 \
  --location=us-central-1 \
  --network=ronggur-my-id \
  --http-port=8080 \
  --instance-type=datumcloud/d1-standard-2 \
  --runtime-class=general-purpose \
  --yes \
  --project=personal-project-86e0525b
```

Plugin `compute v0.8.0` punya `--http-port`, `--runtime-class`, `--network`,
dan `--instance-type`. Tidak punya `--env`. Server Go sudah listen di `:8080`,
jadi `HOST=::` yang dibutuhkan app lain tidak berlaku di sini.

Yang teramati:

- Network `ronggur-my-id` Ready segera setelah apply.
- Rollout selesai dalam 14 detik. Instance
  `ronggur-my-id-default-us-central-1-0` berstatus `Available`, alamat
  `fd20:0:8::1:0:0/96`.
- NetworkService: 1 member, 1 sehat, `serving: true` di `us-central-1`.
- HTTPProxy programmed, sertifikat wildcard terbit. URL:
  `https://avenue-shark-tjrc6.datumproxy.net`.
- DNS publik: A `NXDOMAIN`, AAAA `2607:ed40:20::1` dan `2607:ed40:10::1`.
- TCP ke kedua alamat itu timeout dari laptop tes. IPv6 laptop yang sama juga
  tidak mencapai internet, jadi curl lokal bukan bukti. Fetch dari luar ikut
  timeout, jadi badan halaman belum terkonfirmasi.
- CLI menulis `workload.yaml` di root repo (dump dengan `managedFields`).
  File itu dihapus; jangan di-commit.

Pembaruan 2026-09-29: A record untuk hostname datumproxy sekarang ada
(`67.14.164.1`, `67.14.165.1`), dan halaman terkonfirmasi lewat IPv4.

## 8. ronggur.my.id dipindah ke Datum compute (2026-09-29)

Sebelumnya `@` dan `www` di zone `ronggur-my-id-38gvr7` adalah A record ke
`45.77.171.60`. Domain `ronggur.my.id` sudah Verified sejak 2026-08-12, jadi
tidak ada langkah verifikasi. Langkah yang berlaku ada di
[docs/deploy.md](docs/deploy.md) bagian "Domain ronggur.my.id".

Urutan yang dijalankan:

1. `spec.hostnames: [ronggur.my.id, www.ronggur.my.id]` ke HTTPProxy lewat
   server-side apply dengan field manager `ronggur`. Controller compute hanya
   memiliki `spec.rules`, jadi keduanya tidak bertabrakan. Kedua hostname
   langsung `Claimed`, `HostnamesVerified=True`.
2. Datum **membuat sendiri** DNSRecordSet `ronggur-my-id-4f3ad448`: ALIAS
   `www` ke `avenue-shark-tjrc6.datumproxy.net.`, dimiliki Gateway
   `ronggur-my-id`, label `dns.datumapis.com/managed=true`. Record ini tertahan
   di `DNSRecordProgrammed=Pending` karena A record `www` yang lama masih ada.
   Untuk apex tidak dibuatkan apa pun.
3. Sertifikat kedua hostname tertahan `Pending` selama DNS masih ke server
   lama, dan `Programmed` HTTPProxy turun ke `False`. URL datumproxy tetap 200
   selama itu.
4. Recordset `ronggur-my-id-38gvr7-a` dihapus. Record `www` otomatis langsung
   `RecordCreated` dan sertifikatnya terbit.
5. ALIAS `@` dipasang dari `datum/dns.yaml` (recordset
   `ronggur-my-id-38gvr7-apex`). Nameserver Datum langsung menjawab A dan AAAA
   milik proxy; sertifikat apex terbit kurang dari satu menit kemudian.
   Antara langkah 4 dan 5 apex tidak punya record, beberapa menit.

Hasil: kedua hostname 200 lewat IPv4 dengan sertifikat Let's Encrypt
(`CN=ronggur.my.id`, issuer `YR1`), 1.1.1.1 dan 8.8.8.8 sudah menjawab alamat
Datum. IPv6 tidak bisa dites dari laptop ini (§7).

Yang perlu diingat:

- Kalau diulang, rencanakan langkah 1 → 4 → 5 berturut-turut. Menunggu
  sertifikat sebelum mengganti DNS tidak ada gunanya: sertifikat baru terbit
  setelah DNS mengarah ke Datum.
- `destroy` + deploy ulang membuat HTTPProxy baru tanpa `hostnames`, dan
  hostname datumproxy-nya bisa berganti. `datum/dns.yaml` lalu harus
  diperbarui juga.
- TTL A record lama 14400 detik. Server lama sebaiknya tetap hidup sampai
  sekitar 2026-09-29 23:00 WIB.
