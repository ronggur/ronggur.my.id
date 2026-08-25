# Catatan kerja: menyiapkan ronggur.my.id untuk Datum Compute

Tanggal: 2026-08-19. Sumber acuan: `compute-handbook` (README, getting-started,
integrating-your-app, reference/limits register, api-reference,
ci-cd-and-operations, troubleshooting). Kode `L<angka>` merujuk baris limits
register di `compute-handbook/reference.md`.

Panduan langkah demi langkah tanpa skrip ada di
[deploy-manual.md](deploy-manual.md); runbook ringkas dengan skrip ada di
[deploy/datum/README.md](deploy/datum/README.md).
File ini adalah catatan **apa yang dikerjakan, kenapa, dan apa hasil tesnya**.

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

### Belum diuji sama sekali

Deploy ke Datum dan jembatan ingress (Gateway → EndpointSlice → HTTPRoute).
Mode `unikernel` di `publish-image.yml` juga belum pernah jalan — ia menunggu
kredensial `index.unikraft.io`.

## 5. Keadaan akun

> **Update 2026-08-25:** test aktif sekarang jalan di project baru
> **`website-w7zf79`**, di environment **staging** `cloud.datum.net` (dibuat
> user, bukan `personal-project-86e0525b` yang dicek di bawah). Admission
> mengonfirmasi kota yang didukung project ini adalah **`DFW`** saja —
> `workload.yaml` sudah disesuaikan. Belum dicek: nama organisasi, quota,
> dan apakah entitlement compute-nya sudah Active (deploy pertama sempat
> membuat `Network default` otomatis, jadi setidaknya lolos admission).

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

1. `kraft login` dengan akun Unikraft yang punya akses base-compat. Kalau
   belum punya, minta akses enterprise ke Unikraft/Datum — ini blocker nomor
   satu (L51).
2. `deploy/datum/scripts/build-push.sh` → push ke `ghcr.io/ronggur/ronggur-my-id`.
   Setelah push pertama, set package-nya **Public** di GitHub, kalau tidak cell
   tidak bisa pull (L9).
3. Pin digest yang dilaporkan skrip ke `overlays/production/kustomization.yaml`.
4. `datumctl ctx use personal-org-86e0525b/personal-project-86e0525b`, lalu
   opsional `compute-doctor.py preflight` untuk memastikan hijau.
5. `deploy/datum/scripts/deploy.sh` → tonton rollout.
6. `deploy/datum/scripts/verify.sh` → curl langsung ke `externalIP:8080`.
   Kalau instance terjadwal tapi stall di boot tanpa error, curiga cell gagal
   pull image — periksa package ghcr sudah publik.
7. `kubectl apply -f deploy/datum/ingress/gateway.yaml`,
   `repoint-ingress.sh`, `kubectl apply -f deploy/datum/ingress/httproute.yaml`,
   baca hostname kanonik dari status Gateway, curl.
8. Opsional, kalau build dari laptop mulai merepotkan: jalankan
   `.github/workflows/publish-image.yml` secara manual (butuh secret
   `KRAFT_USER` dan `KRAFT_TOKEN`). Workflow itu hanya mem-build dan push
   image — deploy tetap manual dari terminal, sesuai keputusan untuk tidak
   memakai CI sebagai jalur deploy.

Ingat sepanjang operasi: **setiap deploy, restart, dan rollback wajib diikuti
`repoint-ingress.sh`** — tidak ada apa pun di platform yang memperbarui
EndpointSlice untukmu (L8), dan gejalanya adalah semua objek Datum hijau
sementara klien kena 502.
