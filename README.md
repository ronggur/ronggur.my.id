# ronggur.my.id
Datum compute

## Flappy Bird game

- `flappy.html` — versi canvas murni, tanpa dependency.
- `index.html` — versi sprite, pakai assets dari [samuelcust/flappy-bird-assets](https://github.com/samuelcust/flappy-bird-assets) (folder `flappy-bird-assets/`, MIT License).

## Deploy ke Datum Compute

Situs ini disiapkan untuk jalan sebagai unikernel di Datum Compute:

- `Kraftfile` + `Dockerfile.unikraft` — image unikernel (server statis Go,
  `index.html` + `flappy-bird-assets/`). `Dockerfile` nginx tetap untuk lokal.
- `project.yaml` — manifest project Datum (`personal-project-86e0525b`; project-nya sudah ada, jadi tidak perlu di-apply).
- `deploy/datum/` — `Workload`, overlay digest pin, manifest ingress, dan skrip
  build/deploy/re-point/verify.
- `.github/workflows/publish-image.yml` — manual-dispatch, dua mode: `rootfs`
  (Docker image biasa, tanpa secret) dan `unikernel` (butuh kredensial
  index.unikraft.io). Tidak men-deploy; deploy tetap dari terminal.

Tiga dokumen, dari yang paling detail:

- [deploy-manual.md](deploy-manual.md) — panduan langkah demi langkah, semua
  perintah `datumctl`/`kraft`/`kubectl` diketik langsung tanpa skrip, lengkap
  dengan output yang diharapkan dan tabel error.
- [deploy/datum/README.md](deploy/datum/README.md) — runbook ringkas dengan skrip.
- [compute.md](compute.md) — catatan persiapan: keputusan desain, hasil test,
  dan apa yang masih memblokir.

Uji server yang sama dengan yang jalan di produksi, secara lokal:

```sh
deploy/datum/scripts/serve-local.sh 8080
```
