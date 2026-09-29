# ronggur.my.id
Datum compute

## Flappy Bird game

- `flappy.html` — versi canvas murni, tanpa dependency.
- `index.html` — versi sprite, pakai assets dari [samuelcust/flappy-bird-assets](https://github.com/samuelcust/flappy-bird-assets) (folder `flappy-bird-assets/`, MIT License).

## Deploy ke Datum Compute

- [docs/deploy.md](docs/deploy.md) — build image dan deploy.
- `Dockerfile` nginx tetap untuk lokal. `Dockerfile.unikraft` adalah image yang di-push.

```sh
cd server && PORT=8080 SITE_ROOT=.. go run .
```
