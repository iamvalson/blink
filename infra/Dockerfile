# syntax=docker/dockerfile:1
FROM golang:1.26.0-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -o /out/blink-api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -o /out/blink-worker ./cmd/worker \
 && CGO_ENABLED=0 go build -trimpath -o /out/blink-migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -o /out/blink-keygen ./cmd/keygen

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 blink \
 && useradd --uid 10001 --gid blink --create-home blink
WORKDIR /app
USER 10001:10001

FROM runtime AS auth-runtime
USER root
RUN mkdir -p /var/lib/blink && chown blink:blink /var/lib/blink
USER 10001:10001

FROM auth-runtime AS api
COPY --from=build /out/blink-api /usr/local/bin/blink-api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/blink-api"]

FROM runtime AS worker
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg \
 && rm -rf /var/lib/apt/lists/* \
 && ffprobe -version >/dev/null
COPY --from=build /out/blink-worker /usr/local/bin/blink-worker
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/blink-worker"]

FROM runtime AS migrate
COPY --from=build /out/blink-migrate /usr/local/bin/blink-migrate
ENTRYPOINT ["/usr/local/bin/blink-migrate"]

FROM auth-runtime AS keygen
COPY --from=build /out/blink-keygen /usr/local/bin/blink-keygen
ENTRYPOINT ["/usr/local/bin/blink-keygen"]
