# Deploy ronggur.my.id ke Datum Compute

Situs ini statis: HTML + assets, tanpa state, satu port HTTP. Enam pertanyaan
qualify di handbook semuanya "ya", jadi ini kasus yang memang muat hari ini —
sama seperti contoh milo-os.com.

Yang tidak ikut pindah: `Dockerfile` nginx di root. Cell Datum boot unikernel
lewat kraftlet, dan output `docker build` biasa **tidak boot** di sana (L10).
Karena itu ada artifact kedua — `Kraftfile` + `Dockerfile.unikraft` — yang
membungkus server statis Go ke dalam unikernel. Docker/nginx tetap dipakai
untuk pengembangan lokal; keduanya tidak saling menggantikan.

Semua rujukan `L<angka>` di bawah menunjuk limits register di
`compute-handbook/reference.md`.

Halaman ini memakai skrip. Kalau kamu ingin mengetik sendiri tiap perintah
`datumctl` — disarankan untuk test pertama — pakai
[deploy-manual.md](../../deploy-manual.md) di root repo.

## Isi folder ini

| Path | Fungsi |
|---|---|
| `server/main.go` | Server statis, static-linked, `PORT` (default 8080), `SITE_ROOT` (default `/site`), plus `/healthz` |
| `base/workload.yaml` | `Workload`: 1 container, `d1-standard-2`, port bernama `http:8080`, placement DFW, `minReplicas: 1` |
| `overlays/production/` | Overlay kustomize untuk digest pin (`images:`) |
| `ingress/gateway.yaml` | Gateway edge, listener HTTPS 443, sertifikat otomatis |
| `ingress/endpointslice.yaml` | Jembatan Gateway → alamat instance. **Dihasilkan skrip**, jangan diedit tangan |
| `ingress/httproute.yaml` | Menyambung Gateway ke EndpointSlice |
| `scripts/` | `build-push.sh`, `deploy.sh`, `repoint-ingress.sh`, `verify.sh`, `serve-local.sh` |

Yang di root repo: `Kraftfile`, `Dockerfile.unikraft`, `project.yaml`, dan
`.github/workflows/publish-image.yml` (opsional, build+push saja).

## Status akun (dicek 2026-08-19, read-only)

Sebagian jalur sudah selesai, jadi jangan ulangi dari nol:

| Hal | Keadaan |
|---|---|
| `datumctl` + plugin compute | Terpasang, sudah login sebagai `rhabibun@datum.net` |
| Organisasi | `personal-org-86e0525b` |
| Project `ronggur-my-id` | **Sudah ada** — `datumctl apply -f project.yaml` tidak perlu diulang |
| Entitlement compute di `ronggur-my-id` | **Belum diminta** (`serviceentitlements`: No resources found) → STOP gate satu masih di depan |
| `personal-project-86e0525b` | Sudah entitled: locationbindings `dfw`, `iad`, `sjc` semua Available |

**Keputusan: test pertama jalan di `personal-project-86e0525b`** (project itu
sudah entitled, jadi kedua STOP gate terlewati). `project.yaml` sudah diarahkan
ke sana. Konsekuensinya: langkah 3 dan 5 di bawah dilewati, dan workload test
hidup berdampingan dengan isi project pribadi — isolasi di Datum per project,
bukan per namespace (L15), jadi kalau nanti serius pindahkan ke project sendiri.

`project.yaml` **tidak perlu di-apply**: project-nya sudah ada. Meng-apply-nya
hanya akan menimpa annotation `kubernetes.io/description` project pribadi
dengan "ronggur.my.id".

## Sebelum mulai: dua STOP gate

Dua tunggu yang tidak bisa dipercepat, rencanakan di sekitarnya:

1. **Persetujuan entitlement** (langkah 5). Manusia di sisi Datum yang
   menyetujui; tidak ada lead time yang dipublikasikan, dan bentuk `--wait`
   menyerah setelah 30 menit per percobaan (aman, boleh diulang). Hanya
   **owner** organisasi yang boleh membuatnya — 403 di sini artinya cari owner.
2. **Engagement** (langkah 7). Setelah entitlement Active, deploy masih bisa
   ditolak dengan `consumer project "…" not engaged: cluster not found`.
   Di verification run handbook ini butuh ±10,5 menit setelah Active. Ini
   tunggu-lalu-ulang, bukan salah konfigurasi — jangan ubah apa pun.

Kerjakan langkah build image (langkah 6) selama menunggu; build tidak butuh
apa pun dari project.

## Jalur sekali-jalan

```sh
# 1  [portal] Daftar di https://cloud.datum.net, selesaikan wizard onboarding
#    (organisasi, kontak billing, payment method). Tanpa ini `datumctl login`
#    menolak memberi working context.

# 2  Pasang CLI dan masuk
brew tap datum-cloud/homebrew-tap
brew trust --formula datum-cloud/homebrew-tap/datumctl
brew install datumctl
datumctl login
datumctl whoami

# 3  Pilih project. DILEWATI untuk test pertama — project sudah ada dan entitled.
datumctl ctx use personal-org-86e0525b/personal-project-86e0525b
#    Untuk project baru: isi project.yaml, `datumctl apply -f project.yaml
#    --organization <org>`, `datumctl ctx refresh`, lalu `ctx use`.

# 4  Pasang plugin compute + arahkan kubectl ke project control plane
datumctl plugin install compute           # sudah terpasang di mesin ini
datumctl auth update-kubeconfig --project personal-project-86e0525b

# 5  STOP GATE SATU — minta entitlement. DILEWATI: personal-project sudah Active.
datumctl services enable compute
datumctl services enable networking.datumapis.com     # sambil menunggu; tanpa approval
datumctl services enable dns.networking.miloapis.com
datumctl services enable ipam.miloapis.com
datumctl services list                    # tunggu Compute jadi Active

# 6  Build image unikernel (kerjakan selama menunggu langkah 5)
docker info                               # kraft butuh container runtime + BuildKit
kraft login                               # WAJIB: base-compat ada di index.unikraft.io
deploy/datum/scripts/build-push.sh
#    Push ke ghcr.io publik: cell harus bisa pull tanpa credential per-workload (L9).
#    Pastikan package ghcr-nya diset Public setelah push pertama.

# 7  STOP GATE DUA — engagement. Ulangi deploy sampai tidak ditolak lagi.

# 8  Cek quota dan kota yang boleh dipakai
datumctl compute quota
datumctl get locationbindings -o wide     # --city harus salah satu dari sini
#    Kalau DFW tidak muncul, ganti cityCodes di base/workload.yaml.

# 9  Deploy
cd deploy/datum/overlays/production && kustomize edit set image \
  "ghcr.io/ronggur/ronggur-my-id=ghcr.io/ronggur/ronggur-my-id@sha256:<digest>" && cd -
deploy/datum/scripts/deploy.sh

# 10 Verifikasi langsung ke instance (external IP = NAT 1:1 ke instance)
deploy/datum/scripts/verify.sh

# 11 Expose lewat edge
kubectl apply -f deploy/datum/ingress/gateway.yaml
deploy/datum/scripts/repoint-ingress.sh          # isi EndpointSlice + apply
kubectl apply -f deploy/datum/ingress/httproute.yaml
kubectl get gateway ronggur-my-id-gw -o jsonpath='{.status.addresses[0].value}'
curl https://<hostname>/
```

Preflight otomatis sebelum langkah 9 (opsional, read-only):

```sh
python3 ~/Works/Datum/compute-handbook/tools/compute-doctor.py preflight --project personal-project-86e0525b
```

`smoke` di tool yang sama men-deploy canary sendiri lalu menghapusnya — pakai
itu kalau ingin membuktikan jalur platform sebelum mengirim situs sungguhan.

## Operasi harian

```sh
deploy/datum/scripts/deploy.sh              # rilis (render overlay + deploy -f)
deploy/datum/scripts/repoint-ingress.sh     # WAJIB setelah tiap roll (L8)
deploy/datum/scripts/verify.sh              # gate: curl instance + hostname

datumctl compute workloads                  # health per workload
datumctl compute instances --workload ronggur-my-id
datumctl compute rollout ronggur-my-id      # ikuti rollout yang sedang jalan
datumctl compute restart ronggur-my-id      # roll tanpa ubah spec (mis. setelah Secret)
datumctl compute scale ronggur-my-id --min 2
datumctl compute destroy ronggur-my-id      # satu-satunya cara berhenti pakai quota (L4)
```

Rollback dua langkah: revert digest di `overlays/production/kustomization.yaml`,
jalankan `deploy.sh`, lalu `repoint-ingress.sh`. Rollback yang melewatkan
re-point meninggalkan semua objek Datum hijau sementara klien tetap kena 502.

## Yang perlu diketahui sebelum percaya pada setup ini

- **Tidak ada log, exec, SSH, atau console** (L6). Satu-satunya sinyal adalah
  status condition, audit log, dan curl ke endpoint sendiri.
- **Tidak ada probe** (L5). `Ready=True` berarti platform sudah memprogram
  instance, bukan berarti proses melayani. Pasang uptime monitor eksternal ke
  hostname.
- **EndpointSlice tidak pernah diperbarui platform** (L8). Setiap deploy,
  restart, dan rollback menuntut `repoint-ingress.sh`.
- **`minReplicas: 1` berarti tiap rollout memutus layanan** (L21, L43): tidak
  ada surge, instance diganti satu per satu. Naikkan ke 2 kalau situs ini
  mulai dipakai orang.
- **Tag mutable tidak me-roll apa pun** (L52). Selalu deploy dengan digest.
- **Jangan pakai bentuk flags `datumctl compute deploy <name> --image …`** —
  ia menulis ulang seluruh spec dari flags dan menimpa `workload.yaml` di
  direktori kerja (L54).

## Yang belum terbukti — perlakukan run pertama sebagai eksperimen

Handbook menandai bagian ini `pending-live`; artinya ditulis dari source code
platform, belum pernah diamati jalan:

| Status | Hal | Catatan |
|---|---|---|
| ✅ Terbukti | Registry ghcr.io: push dari CI tanpa secret, package publik, bisa ditarik anonim | `ghcr.io/ronggur/ronggur-my-id@sha256:abd98747…` — manifest, tags, dan config blob semua 200 tanpa kredensial (2026-08-19) |
| ✅ Terbukti | Rootfs `Dockerfile.unikraft` terbentuk dan melayani | `docker build --platform linux/amd64` + `docker run` → `/`, sprite, wav, favicon, `/healthz` semua 200 (2026-08-19) |
| ⛔ **Blocked sekarang** | `kraft pkg` tidak bisa mengambil runtime `base-compat` | `could not find runtime 'index.unikraft.io/official/base-compat:latest' (kraftcloud/x86_64)`. Anonim ditolak; butuh `kraft login` dengan akun yang punya akses base enterprise (L51). Ini penghalang nyata sebelum deploy apa pun |
| ❓ Belum terbukti | Flag `kraft pkg` dan relabel `kraftcloud/x86_64` | `--plat kraftcloud --arch x86_64` valid menurut `kraft pkg --help` v0.12.15, tapi belum pernah menghasilkan package |
| ❓ Belum terbukti | Cell benar-benar bisa pull image | Kalau gagal: instance terjadwal lalu stall di boot tanpa error — persis yang terjadi pada milo-os.com |
| ❓ Belum terbukti | Instance mencapai `Ready=True` dan melayani | `verify.sh` akan gagal di curl langsung |
| ❓ Belum terbukti | Jembatan ingress end to end | Belum jelas juga external IP atau network IP yang benar masuk slice (L7) |

Kalau instance tidak pernah muncul dan workload berkata
`NoAvailablePlacements` tanpa error lain: itu sisi platform. Tunggu beberapa
menit, lalu kumpulkan bukti dengan
`compute-doctor.py diagnose --project personal-project-86e0525b --workload ronggur-my-id`
dan kirim ke support@datum.net. Jangan bongkar-pasang ulang.
