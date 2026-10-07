# Image untuk status.ronggur.my.id (Datum Compute, general-purpose).
# Konteks build adalah folder status/ ini:
#   docker build --platform linux/amd64 --build-arg COMMIT=$(git rev-parse --short=7 HEAD) -t status:test status
#
# Pola sama dengan ../Dockerfile.unikraft: build stage lalu FROM scratch,
# binary statis, x86_64.
FROM --platform=linux/amd64 golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY main.go index.html ./

ARG COMMIT=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags="-s -w -X main.version=${COMMIT}" -o /out/server .

# Binary dinamis akan mati saat boot tanpa pesan di image scratch.
RUN if ldd /out/server >/dev/null 2>&1; then \
      echo "ERROR: /out/server is dynamically linked"; exit 1; \
    fi

# Sertifikat CA dibutuhkan probe TLS dan HTTP ke luar; image scratch tidak
# membawanya.
FROM --platform=linux/amd64 scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/server /server

EXPOSE 8080
CMD ["/server"]
