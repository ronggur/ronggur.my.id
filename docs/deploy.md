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
`ghcr.io/ronggur/ronggur-my-id@sha256:abd987474d45c336aeb2607e10fef0830556b6241158daf16d0d80b7981b4ba4`

## Deploy

`--project` di belakang.

```bash
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

URL: https://avenue-shark-tjrc6.datumproxy.net

DNS-nya hanya IPv6. Perintah ini menulis `workload.yaml` di direktori kerja.
Jangan di-commit.

```bash
datumctl compute instances --workload=ronggur-my-id --project=personal-project-86e0525b
datumctl compute destroy ronggur-my-id --project=personal-project-86e0525b
```

Kalau network-nya hilang, buat sekali:

```yaml
apiVersion: networking.datumapis.com/v1alpha
kind: Network
metadata:
  name: ronggur-my-id
  namespace: default
spec:
  ipFamilies: [IPv6]
  ipam:
    mode: Auto
  mtu: 1440
```

```bash
datumctl apply -f network.yaml --project=personal-project-86e0525b
```

Tanpa `--network=ronggur-my-id`, workload menempel ke `default`, yang di
project ini IPv4 dan tidak pernah dapat alamat.

## Lokal

```bash
cd server && PORT=8080 SITE_ROOT=.. go run .
```
