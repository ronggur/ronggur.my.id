# Image for status.ronggur.my.id (Datum Compute, general-purpose).
# The build context is this status/ folder:
#   docker build --platform linux/amd64 --build-arg COMMIT=$(git rev-parse --short=7 HEAD) -t status:test status
#
# Same pattern as ../Dockerfile.unikraft: a build stage, then FROM scratch,
# static binary, x86_64.
FROM --platform=linux/amd64 golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY main.go index.html ./

ARG COMMIT=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags="-s -w -X main.version=${COMMIT}" -o /out/server .

# A dynamic binary dies silently at boot in a scratch image.
RUN if ldd /out/server >/dev/null 2>&1; then \
      echo "ERROR: /out/server is dynamically linked"; exit 1; \
    fi

# The TLS and outbound HTTP probes need CA certificates; the scratch image
# does not ship them.
FROM --platform=linux/amd64 scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/server /server

EXPOSE 8080
CMD ["/server"]
