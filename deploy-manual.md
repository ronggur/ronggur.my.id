# Panduan manual: deploy ronggur.my.id ke Datum Compute

Panduan ini menempuh jalur penuh **tanpa skrip apa pun** — setiap perintah
`datumctl`, `kraft`, dan `kubectl` diketik langsung. Skrip di
`deploy/datum/scripts/` sengaja diabaikan di sini; pakai panduan ini untuk test
pertama supaya kelihatan persis apa yang terjadi di tiap langkah, dan baru
pindah ke skrip kalau langkahnya sudah membosankan.

Format tiap langkah: **perintah → output yang diharapkan → kalau gagal**.

Catatan kejujuran: output yang ditandai *(transkrip handbook)* datang dari
verification run Datum di staging 2026-08-09. Yang ditandai *(dari source)*
belum pernah diamati jalan oleh siapa pun — termasuk seluruh Langkah 8-11.
Kode `L<angka>` merujuk limits register di `compute-handbook/reference.md`.

---

## Peta jalan

```
1  cek tools           2  pilih project      3  cek entitlement/quota/kota
        │                     │                        │
        └─────────────────────┴────────────────────────┘
                              ▼
4  build image unikernel  →  5  tulis workload.yaml  →  6  deploy
                              ▼
7  baca status  →  8  curl langsung ke instance
                              ▼
9  pasang ingress  →  10 curl lewat hostname
                              ▼
11 operasi harian (update, restart, scale, rollback, destroy)
```

Dua STOP gate handbook (approval entitlement dan engagement) **sudah terlewati**
untuk `personal-project-86e0525b` — project itu sudah entitled. Kalau kamu
pindah ke project baru, keduanya muncul lagi; Langkah 3 menjelaskan cara
mengenalinya.

---

## Langkah 1 — Pastikan tools siap

```sh
datumctl --version
datumctl compute --help
kraft version
docker info
```

**Yang diharapkan:** keempatnya menjawab. `datumctl compute --help` harus
menampilkan daftar subcommand (`deploy`, `destroy`, `instances`, `quota`,
`restart`, `rollout`, `scale`, `workloads`).

**Kalau gagal:**

| Gejala | Aksi |
|---|---|
| `datumctl: command not found` | `brew tap datum-cloud/homebrew-tap && brew trust --formula datum-cloud/homebrew-tap/datumctl && brew install datumctl` |
| `datumctl compute` tidak dikenal | `datumctl plugin install compute` — harusnya muncul `Installed compute v0.8.0-dev.7 from datum [official]` |
| `docker info` error | Nyalakan Docker Desktop. `kraft` membangun rootfs lewat BuildKit dan gagal sebelum mulai kalau tidak ada container runtime |
| `kraft: command not found` | `brew install kraftkit` |

Jangan cari `datumctl compute logs`, `rollout undo`, atau `datumctl config set` —
teks bantuan menjanjikan ketiganya, tidak satu pun ada (L11).

---

## Langkah 2 — Login dan pilih project

```sh
datumctl whoami
```

**Yang diharapkan:**

```
User:         Ronggur Habibun (rhabibun@datum.net)
Onboarding:   ready
Context:      personal-org-86e0525b/personal-project-86e0525b
Organization: Ronggur Habibun (personal-org-86e0525b)
Project:      Personal Project (personal-project-86e0525b)
```

Kalau `Context` belum menunjuk project yang benar:

```sh
datumctl ctx list
datumctl ctx use personal-org-86e0525b/personal-project-86e0525b
```

Setelah ini semua perintah boleh tanpa `--project`; context yang membawanya.

Arahkan juga `kubectl` ke project control plane sekarang — nanti dipakai di
Langkah 9 untuk `Gateway`, `EndpointSlice`, dan `HTTPRoute`, tiga Kind yang
tidak dicakup plugin compute:

```sh
datumctl auth update-kubeconfig --project personal-project-86e0525b
kubectl config current-context
```

**Yang diharapkan:** context bernama `datum-project-personal-project-86e0525b`.

**Kalau gagal:** `credentials for user '…' not found in keyring` berarti sesi
hilang dari keyring OS — `datumctl login` sekali lagi.

---

## Langkah 3 — Cek entitlement, quota, dan kota

```sh
datumctl compute quota
```

**Yang diharapkan** *(transkrip handbook)*:

```
Quota for project personal-project-86e0525b

RESOURCE    UNIT        LIMIT   USED   AVAILABLE   USAGE
Workloads   workloads   1000    0      1000        [--------------------]   0%
Instances   instances   10      0      10          [--------------------]   0%
vCPUs       vCPUs       40      0      40          [--------------------]   0%
Memory      MiB         81920   0      81920       [--------------------]   0%
```

Empat baris ini baru ada setelah entitlement Active. Angka nol semua atau
perintah malah menawarkan "Would you like to request access?" artinya project
ini belum entitled — jawab **no** kalau cuma mau melihat (menjawab ya membuat
`ServiceEntitlement`), lalu urus STOP gate satu:

```sh
datumctl services enable compute        # hanya owner organisasi yang boleh
datumctl services status compute.datumapis.com
```

Approval dilakukan manusia di sisi Datum tanpa batas waktu yang dipublikasikan
(L29). Bentuk `--wait` menyerah setelah 30 menit per percobaan — aman diulang.

Lalu lihat kota yang boleh dipakai:

```sh
datumctl get locationbindings -o wide
```

**Yang diharapkan** (project ini, dicek 2026-08-19):

```
NAME   LOCATION   CLASS           AVAILABLE   AGE
dfw    dfw        datum-managed   True        23h
iad    iad        datum-managed   True        23h
sjc    sjc        datum-managed   True        23h
```

Kolom NAME adalah kode kota versi huruf kecil; di manifest nanti tulis versi
huruf besar (`DFW`). Kalau kota yang kamu mau tidak ada, itu binding sisi
platform — tidak ada peran user yang bisa membuatnya (L30), kirim ke
support@datum.net.

---

## Langkah 4 — Bangun image unikernel dan push

Docker image nginx yang ada di repo **tidak boot** di Datum: cell menjalankan
unikernel lewat kraftlet (L10). Yang dibangun di sini adalah artifact terpisah
dari `Kraftfile` + `Dockerfile.unikraft`.

### 4a. Login ke registry base image

```sh
kraft login
```

**Wajib.** Runtime `base-compat` diambil dari `index.unikraft.io`, yang menolak
semua pull anonim, dan akses ke base image enterprise itu sendiri dikontrol
(L51). Tanpa ini Langkah 4c berhenti dengan:

```
level=error msg="could not package: could not find runtime
  'index.unikraft.io/official/base-compat:latest' (kraftcloud/x86_64)"
```

(Itu persis yang terjadi waktu panduan ini disiapkan — jadi kalau kamu belum
punya akses base, mintalah dulu ke Unikraft/Datum. Tidak ada jalan memutar.)

### 4b. Periksa rootfs-nya benar dulu (opsional, murah)

```sh
docker build --platform linux/amd64 -f Dockerfile.unikraft -t rootfs-check .
docker run --rm --platform linux/amd64 -p 8080:8080 rootfs-check
```

Di terminal lain:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/
curl http://127.0.0.1:8080/healthz
```

**Yang diharapkan:** `200` dan `ok`. Ini membuktikan binary-nya static, file
situs ada di `/site`, dan servernya melayani — semua sebelum menyentuh Datum.
Hentikan dengan Ctrl-C.

### 4c. Package dan push

Untuk push ke ghcr.io dari laptop kamu butuh **Personal Access Token (classic)**
dengan scope `write:packages` — `GITHUB_TOKEN` hanya ada di dalam GitHub
Actions, tidak di terminal. Buat di
`https://github.com/settings/tokens` lalu:

```sh
export IMAGE=ghcr.io/ronggur/ronggur-my-id
export TAG=$(git rev-parse --short HEAD)
export GHCR_PAT=ghp_xxxxxxxxxxxx            # PAT classic, scope write:packages

echo "$GHCR_PAT" | docker login ghcr.io -u ronggur --password-stdin
crane auth login ghcr.io -u ronggur -p "$GHCR_PAT"

kraft pkg --name "$IMAGE:$TAG" --plat kraftcloud --arch x86_64 --push .
```

`--plat kraftcloud --arch x86_64` yang menaruh entry platform `kraftcloud/x86_64`
yang dibutuhkan kraftlet ke dalam OCI index.

**Verifikasi hasil push:**

```sh
crane manifest "$IMAGE:$TAG" | jq '.manifests[].platform'
crane digest "$IMAGE:$TAG"
```

**Yang diharapkan:** salah satu platform adalah `kraftcloud/x86_64`, dan digest
`sha256:…` tercetak. **Catat digest itu** — dipakai di Langkah 5.

### 4d. Jadikan package publik

Buka `https://github.com/users/ronggur/packages/container/ronggur-my-id/settings`
→ Change visibility → **Public**.

Ini bukan opsional, dan tidak ada hubungannya dengan CI: **cell yang menarik
image, bukan laptopmu**. Meski build dan push dilakukan dari terminal sendiri,
image harus duduk di registry yang bisa ditarik cell tanpa credential
per-workload. `imagePullSecrets` ada di API tapi pipeline-nya tidak pernah
mengumpulkan pull secret, jadi registry privat gagal (L9), dan
`index.unikraft.io` menolak semua pull anonim.

Registry publik mana pun sebenarnya boleh — ghcr.io dipilih karena repo ini
sudah di GitHub. Buktikan dari sesi yang tidak login:

```sh
docker logout ghcr.io
crane manifest "$IMAGE:$TAG" > /dev/null && echo "bisa dipull anonim"
```

---

## Langkah 5 — Tulis workload.yaml

Ketik file ini sendiri (atau salin dari `deploy/datum/base/workload.yaml` dan
ganti image-nya dengan digest dari Langkah 4c):

```sh
cat > workload.yaml <<'EOF'
apiVersion: compute.datumapis.com/v1alpha
kind: Workload
metadata:
  name: ronggur-my-id
spec:
  template:
    spec:
      runtime:
        resources:
          instanceType: datumcloud/d1-standard-2
        sandbox:
          containers:
            - name: web
              image: ghcr.io/ronggur/ronggur-my-id@sha256:GANTI_DENGAN_DIGEST
              env:
                - name: PORT
                  value: "8080"
                - name: SITE_ROOT
                  value: /site
              ports:
                - name: http
                  port: 8080
                  protocol: TCP
      networkInterfaces:
        - network:
            name: default
          networkPolicy:
            ingress:
              - ports:
                  - port: 8080
                from:
                  - ipBlock:
                      cidr: 0.0.0.0/0
  placements:
    - name: us
      cityCodes:
        - DFW
      scaleSettings:
        minReplicas: 1
EOF
```

Yang perlu dipahami dari isi file ini:

| Bagian | Kenapa begitu |
|---|---|
| `instanceType: datumcloud/d1-standard-2` | Satu-satunya nilai yang diterima. Jangan tambahkan `resources` per container — admission menolaknya `Forbidden: not implemented` (L1) |
| `image: …@sha256:…` | Pin digest. Tag mutable yang di-push ulang tidak me-roll apa pun karena rollout hanya terpicu perubahan hash template (L52) |
| `ports[].name` | Wajib. Port tanpa nama ditolak admission |
| `networkInterfaces[].network.name: default` | Tiap interface butuh `Network`. Kalau `default` belum ada, lihat catatan di Langkah 6 |
| `networkPolicy.ingress` dengan `ipBlock: 0.0.0.0/0` | Membuka port 8080 ke internet. Hanya `ipBlock` yang didukung — tidak ada selector label/pod/namespace, dan tidak ada aturan egress (L14) |
| `placements[].cityCodes: [DFW]` | Huruf besar, dan harus ada di output Langkah 3 |
| `minReplicas: 1` | Minimal 1; `0` ditolak dengan `must be greater than 0` (L4). Tidak ada scale-to-zero |

Tidak ada `livenessProbe`/`readinessProbe` di API mana pun (L5), dan tidak ada
`Service` (L49) — jangan cari padanannya.

---

## Langkah 6 — Deploy

```sh
datumctl compute deploy -f workload.yaml
```

**Yang diharapkan** *(transkrip handbook, deploy pertama di project yang belum
punya Network)*:

```
  Network "default" does not exist in project personal-project-86e0525b.
  network/default created
Resolving workload "ronggur-my-id" in project personal-project-86e0525b...
  Placement "default": cities=[DFW], min=1
workload/ronggur-my-id created
Waiting for rollout. Ctrl-C to detach (rollout continues in background).

  PLACEMENT   CITY   UPDATED   READY   OLD   PHASE
  us          DFW    0         0       0     Pending
```

Tiga hal yang baru terjadi: `Network` bernama `default` dibuat otomatis (kalau
belum ada — tambahkan `-y` supaya tidak ditanya), `Workload` lolos admission,
dan perintahnya masuk mode menonton rollout.

**Ctrl-C aman**: itu hanya melepas tontonan, rollout jalan terus di server.
Tidak ada flag untuk melewati tontonan ini, dan kalau rollout tidak bisa maju
ia menggantung selamanya — di CI wajib pasang timeout tingkat job.

Menonton lagi kapan saja:

```sh
datumctl compute rollout ronggur-my-id
```

**Kalau ditolak** — baca Lampiran A; error admission selalu menyebut field-nya
dan nilai yang didukung, dan daftar itu adalah katalog hidup, bukan salinan
dokumentasi.

> **Jalur cepat yang sebaiknya dihindari.** Bentuk flags ini juga ada:
> ```sh
> datumctl compute deploy ronggur-my-id --image ghcr.io/ronggur/ronggur-my-id:latest \
>   --city DFW --min 1 --port 8080 --yes
> ```
> Ia menulis `workload.yaml` ke direktori kerja **menimpa file yang sudah ada**,
> dan membangun ulang seluruh spec dari flags saja — env, network policy, dan
> placement tambahan hilang (L54). Semua `--city` juga digabung jadi satu
> placement bernama `default` (L25). Pakai hanya untuk workload sekali buang.

> **Yang tidak bekerja:** `deploy -f -` (tidak ada jalur stdin, L53), file
> multi-dokumen (hanya dokumen pertama yang dibaca — sisanya hilang tanpa pesan,
> L39), dan `envFrom` (didrop diam-diam oleh decoder plugin — pakai
> `datumctl apply -f` untuk itu).

---

## Langkah 7 — Baca status

```sh
datumctl compute workloads
```

**Yang diharapkan:** satu baris dengan kolom NAME, HEALTH, READY, UP-TO-DATE,
PLACEMENTS, IMAGE, AGE. HEALTH berisi `Available`, `Degraded`, `Unavailable`,
atau `Unknown`.

```sh
datumctl compute workloads describe ronggur-my-id
datumctl compute instances --workload ronggur-my-id
```

**Yang diharapkan** *(dari source)*: satu instance bernama
`ronggur-my-id-us-dfw-0` — polanya `<workload>-<placement>-<kota>-<ordinal>`.
Kolom STATUS berisi `Available`, `Starting`, `Pending (<reason>)`, atau
`Failed (image unavailable | crashing | configuration error)`.

Kalau ada yang aneh, baca condition mentahnya — `describe` merangkum, sedangkan
yang perlu dicocokkan adalah string `reason`:

```sh
datumctl get workload ronggur-my-id -o yaml
datumctl get workloaddeployments -o yaml
datumctl compute instances describe ronggur-my-id-us-dfw-0
```

Dua permukaan diagnosis yang bekerja hanyalah **condition** dan **audit log**:

```sh
datumctl activity audit --resource workloads --start-time now-1d
```

Selalu pakai `--resource`, karena view audit mencatat query-nya sendiri.
`datumctl get events` selalu kosong untuk objek compute, dan
`datumctl activity feed` tidak memuat tulisan compute sama sekali (L20).
Timestamp audit dicetak waktu lokal walau berakhiran `Z` — jangan dikorelasikan
mentah-mentah dengan log UTC.

Tidak ada log, exec, SSH, serial console, atau VNC ke instance (L6). Kalau
instance-nya hidup tapi diam, satu-satunya cara tahu adalah curl di Langkah 8.

---

## Langkah 8 — Curl langsung ke instance

```sh
datumctl compute instances describe ronggur-my-id-us-dfw-0
```

Baca blok Network, ambil external IP (di API:
`status.networkInterfaces[0].assignments.externalIP`) — itu NAT 1:1 ke instance.

```sh
curl -v http://<external-ip>:8080/
curl http://<external-ip>:8080/healthz
```

**Yang diharapkan** *(dari source — belum pernah diamati live oleh handbook)*:
HTML halaman Flappy Bird, dan `ok`.

**Kalau menggantung atau ditolak padahal semua `Ready=True`:** itu pola
"hung-but-Ready". `Ready=True` hanya berarti platform selesai memprogram
instance, bukan bahwa prosesmu melayani — tidak ada probe sama sekali (L5).
Curl adalah satu-satunya sinyal kesehatan yang nyata.

---

## Langkah 9 — Pasang ingress

Tiga objek yang kamu tulis sendiri: `Gateway`, `EndpointSlice`, `HTTPRoute`.
Semua lewat `kubectl`, bukan plugin compute. Tidak ada backend yang menunjuk
langsung ke `Workload` — jembatan `EndpointSlice` ini memang satu-satunya jalan
(L7).

### 9a. Gateway

```sh
cat > gateway.yaml <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ronggur-my-id-gw
spec:
  gatewayClassName: datum-external-global-proxy
  listeners:
    - name: https
      port: 443
      protocol: HTTPS
      tls:
        mode: Terminate
        options:
          gateway.networking.datumapis.com/certificate-issuer: auto
      allowedRoutes:
        namespaces:
          from: Same
EOF

kubectl apply -f gateway.yaml
```

Webhook hanya mengizinkan port 80 dan 443, mode TLS `Terminate`, dan melarang
`certificateRefs` — sertifikat diterbitkan untukmu (L18). Jadi bentuk di atas
nyaris satu-satunya yang valid.

### 9b. Baca hostname kanonik

```sh
kubectl get gateway ronggur-my-id-gw -o jsonpath='{.status.addresses[0].value}'
```

**Yang diharapkan:** nama seperti `dua-kata-abcde.datumproxy.net` — diturunkan
dari UID Gateway, jadi tidak bisa ditebak sebelum dibuat; selalu baca dari
status. HTTPS-nya dilayani sertifikat wildcard bersama, tanpa menunggu
penerbitan.

### 9c. EndpointSlice

Isi dengan external IP dari Langkah 8:

```sh
cat > endpointslice.yaml <<'EOF'
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: ronggur-my-id-endpoints
addressType: IPv4
ports:
  - name: http
    protocol: TCP
    appProtocol: http
    port: 8080
endpoints:
  - addresses:
      - <external-ip>
EOF

kubectl apply -f endpointslice.yaml
```

Tiap port **wajib punya `name`**, kalau tidak operator menolak slice-nya.
External IP vs network IP mana yang benar masuk sini masih pertanyaan terbuka
di handbook (L7); external IP adalah bentuk yang dicoba lebih dulu.

### 9d. HTTPRoute

```sh
cat > route.yaml <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ronggur-my-id-route
spec:
  parentRefs:
    - name: ronggur-my-id-gw
  rules:
    - backendRefs:
        - group: discovery.k8s.io
          kind: EndpointSlice
          name: ronggur-my-id-endpoints
          port: 8080
EOF

kubectl apply -f route.yaml
```

Jangan menaruh `spec.hostnames` di route — webhook menolaknya dengan
`hostnames are not permitted` (L19). Hostname tinggal di listener Gateway.

### 9e. Buktikan

```sh
curl -v https://<hostname>/
```

**Yang diharapkan** *(dari source)*: halaman situs lewat TLS yang valid.

Di belakang layar operator menyalin slice-mu ke data plane, memasang headless
Service di depannya, dan mengawasinya — edit slice-nya dan data plane ikut
otomatis, tanpa menyentuh Gateway atau route lagi.

---

## Langkah 10 — Setelah setiap perubahan: re-point ingress

Ini kewajiban permanen, bukan langkah sekali jalan. Update itu delete-and-recreate,
dan **tidak ada apa pun di platform yang menulis alamat instance ke
EndpointSlice** (L8). Gejala kalau lupa: semua objek Datum hijau, klien kena 502.

```sh
# 1. alamat sekarang
datumctl compute instances --workload ronggur-my-id

# 2. bandingkan dengan isi slice
kubectl get endpointslice ronggur-my-id-endpoints -o yaml

# 3. kalau beda, edit endpointslice.yaml lalu
kubectl apply -f endpointslice.yaml

# 4. buktikan lewat pintu depan
curl -fsS https://<hostname>/
```

---

## Langkah 11 — Operasi harian

### Rilis versi baru

```sh
# build & push image baru (Langkah 4), ambil digest
crane digest ghcr.io/ronggur/ronggur-my-id:$TAG

# ganti baris image: di workload.yaml dengan digest baru, lalu
datumctl compute deploy -f workload.yaml
```

Rollout terjadi **hanya** kalau hash template berubah. Digest baru = template
baru = roll. Push ulang tag yang sama = tidak terjadi apa-apa.

Instance diganti satu per satu, ordinal tertinggi dulu, tanpa surge — dengan
`minReplicas: 1` artinya situs mati selama pergantian (L21, L43). Naikkan ke 2
kalau tidak boleh putus:

```sh
datumctl compute scale ronggur-my-id --min 2
```

`scale` menimpa `minReplicas` di **semua** placement; per-placement harus lewat
YAML (L26).

### Restart tanpa ubah spec

```sh
datumctl compute restart ronggur-my-id
datumctl compute restart ronggur-my-id --city DFW    # satu kota saja
```

Dipakai misalnya setelah mengubah `Secret` — instance bisa boot sebelum
Secret-nya sampai, dan env-nya kosong tanpa error apa pun (L16).

### Rollback (dua langkah, tidak ada `rollout undo`)

```sh
# 1. kembalikan baris image: ke digest lama, lalu
datumctl compute deploy -f workload.yaml

# 2. WAJIB
kubectl apply -f endpointslice.yaml    # setelah memperbarui alamatnya
```

Inilah alasan kedua kenapa digest wajib: digest menamai satu build selamanya,
jadi langkah 1 pasti tepat.

### Berhenti dan bersih-bersih

```sh
datumctl compute destroy ronggur-my-id
kubectl delete -f route.yaml -f endpointslice.yaml -f gateway.yaml
```

`destroy` juga satu-satunya cara berhenti memakai quota (L4).

---

## Lampiran A — Error yang mungkin muncul

### Ditolak saat create (salahmu, ada nama field-nya)

| Error | Artinya | Perbaikan |
|---|---|---|
| `consumer project "…" not engaged: cluster not found` | STOP gate dua: platform belum menyambungkan project-mu. Bukan salah konfigurasi | Tunggu ~10 menit, ulangi deploy yang sama persis. Kalau lewat 30 menit, eskalasi |
| `spec.template.spec.runtime.sandbox.containers[0].resources: Forbidden: not implemented` | Ada blok `resources` per container | Hapus; ukuran ditentukan instance type (L1) |
| `runtime.resources: Unsupported value: "…": supported values: "datumcloud/d1-standard-2"` | Instance type salah | Pakai nilai yang disebut error-nya |
| `spec.placements[0].cityCodes[0]: Unsupported value: "LHR": supported values: "DFW"` | Kota tidak terikat ke project | Pakai kota dari daftar di error, atau `datumctl get locationbindings` |
| `… is forbidden: Something went wrong while checking your quota…` | Menyesatkan: biasanya objekmu bukan di namespace `default` | Semua objek compute harus di `default` (L15) |
| `permission to use the network was denied` | Peranmu tidak boleh `use` Network | Minta owner org menaikkan peran ke editor |
| `Error: reading manifest: open -: no such file or directory` | Kamu memakai `deploy -f -` | Tulis file sungguhan (L53) |

### Diterima, tapi tidak ada instance

| Yang terlihat | Artinya | Aksi |
|---|---|---|
| `Available=False/NoAvailablePlacements` beberapa menit pertama | Kondisi default sebelum scheduling selesai | Tunggu beberapa menit |
| `NoAvailablePlacements` **menetap**, WorkloadDeployment ada tapi `status`-nya kosong, `No instances found` | Cell kota itu terputus dari scheduler. **Sisi Datum** | Jangan bongkar ulang. Eskalasi dengan project, workload, kota, blok condition, dan output `No instances found` |
| `Available=False/NetworkNotFound` | Manifest menunjuk Network yang tidak ada | `datumctl get networks`; buat `default` |
| `QuotaNotGranted` / instance `PendingQuota` | Quota habis | `datumctl compute quota`, kecilkan atau minta naik (L28) |
| Jumlah WorkloadDeployment kurang dari placement × kota, tanpa error | Silent placement skip. **Sisi Datum** | Eskalasi |

### Instance ada tapi tidak melayani

| Yang terlihat | Artinya | Aksi |
|---|---|---|
| `Ready=False/ImageUnavailable` | Cell tidak bisa mengambil image — registry privat atau menolak anonim | Pastikan package ghcr-nya publik (Langkah 4d); pull secret tidak sampai ke cell (L9) |
| `Pending` berjam-jam, image-nya Docker image biasa | Admission menerima string image apa pun, tapi cell menjalankan unikernel | Bangun dengan Unikraft toolchain (L10) |
| Admit lalu `Pending` selamanya, base image komunitas | Base enterprise yang dibutuhkan | Minta akses base enterprise (L51) |
| `Ready=False/InstanceCrashing` — boot lalu mati dalam hitungan detik | Entrypoint bukan static, atau `.so` hilang dari image | Untuk repo ini seharusnya tidak terjadi: binary-nya static (dijaga cek `ldd` saat build) |
| `Starting` 30+ menit, `QuotaGranted=True`, tanpa IP | Outage jalur boot sisi cell. **Sisi Datum** | Eskalasi dengan nama instance, kota, dan timeline |
| Semua `Ready=True` tapi endpoint diam | Hung-but-Ready — `Ready` bukan tanda melayani (L5) | Curl endpoint sendiri; pasang uptime monitor eksternal |
| 502 lewat hostname padahal semua hijau | EndpointSlice basi (L8) | Langkah 10 |

### Bahan eskalasi ke support@datum.net

Sertakan: nama project, nama workload, kota, blok `status.conditions` mentah
dari `datumctl get workload … -o yaml`, output `datumctl compute instances`,
dan waktu deploy. Kalau ada `python3`, satu perintah ini merangkumnya:

```sh
python3 ~/Works/Datum/compute-handbook/tools/compute-doctor.py diagnose \
  --project personal-project-86e0525b --workload ronggur-my-id
```

---

## Lampiran B — Ringkasan perintah

```sh
# status
datumctl compute quota
datumctl compute workloads
datumctl compute workloads describe ronggur-my-id
datumctl compute instances --workload ronggur-my-id
datumctl compute instances describe ronggur-my-id-us-dfw-0
datumctl get workload ronggur-my-id -o yaml
datumctl activity audit --resource workloads --start-time now-1d

# ubah
datumctl compute deploy -f workload.yaml
datumctl compute rollout ronggur-my-id
datumctl compute restart ronggur-my-id
datumctl compute scale ronggur-my-id --min 2
datumctl compute destroy ronggur-my-id

# ingress
kubectl apply -f gateway.yaml
kubectl get gateway ronggur-my-id-gw -o jsonpath='{.status.addresses[0].value}'
kubectl apply -f endpointslice.yaml
kubectl apply -f route.yaml
kubectl get endpointslice ronggur-my-id-endpoints -o yaml
```

Kalau langkah-langkah ini sudah terasa hafal, `deploy/datum/scripts/` melakukan
hal yang sama persis dalam empat perintah — dan
[deploy/datum/README.md](deploy/datum/README.md) adalah versi ringkasnya.
