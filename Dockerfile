# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS build
WORKDIR /src
# Keep Go's module and build caches between builds; without them every
# rebuild recompiles everything, including the large generated Jellyfin client.
ENV GOCACHE=/root/.cache/go-build GOMODCACHE=/go/pkg/mod
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/jellex ./cmd/jellex && mkdir -p /out/cache /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/jellex /jellex
# The Plex Web client is downloaded here on first start; mount a volume to keep it.
COPY --from=build --chown=nonroot:nonroot /out/cache /cache
ENV JELLEX_WEB_DIR=/cache/plex-web
# jellex state (the Jellyfin-to-Plex ID map); mount a volume so IDs survive rebuilds.
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV JELLEX_DATA_DIR=/data
EXPOSE 32400
ENTRYPOINT ["/jellex"]
