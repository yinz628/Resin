# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS web-builder
WORKDIR /src/webui

COPY webui/package.json webui/package-lock.json ./
RUN npm ci

COPY webui/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
COPY --from=web-builder /src/webui/dist ./webui/dist

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build -trimpath -tags "with_quic with_wireguard with_grpc with_utls" \
  -ldflags="-s -w \
  -X github.com/Resinat/Resin/internal/buildinfo.Version=${VERSION} \
  -X github.com/Resinat/Resin/internal/buildinfo.GitCommit=${GIT_COMMIT} \
  -X github.com/Resinat/Resin/internal/buildinfo.BuildTime=${BUILD_TIME}" \
  -o /out/resin ./cmd/resin

FROM alpine:3.21 AS obfs-builder
RUN apk add --no-cache \
  autoconf \
  automake \
  build-base \
  git \
  libev-dev \
  libsodium-dev \
  libtool \
  linux-headers \
  pcre-dev

RUN git clone --depth 1 https://github.com/shadowsocks/simple-obfs.git /tmp/simple-obfs \
  && cd /tmp/simple-obfs \
  && git submodule update --init --recursive \
  && ./autogen.sh \
  && ./configure --prefix=/usr/local --disable-documentation \
  && make -j"$(nproc)" \
  && make install \
  && strip /usr/local/bin/obfs-local || true

FROM alpine:3.21
# NOTE: Keep this runtime stage in sync with .github/Dockerfile.release.
# GHCR release images are built from .github/Dockerfile.release, not this file.
RUN apk add --no-cache ca-certificates tzdata su-exec libev libsodium pcre \
  && addgroup -S resin \
  && adduser -S -G resin -h /var/lib/resin resin \
  && mkdir -p /var/cache/resin /var/lib/resin /var/log/resin \
  && chown -R resin:resin /var/cache/resin /var/lib/resin /var/log/resin

COPY --from=go-builder /out/resin /usr/local/bin/resin
COPY --from=obfs-builder /usr/local/bin/obfs-local /usr/local/bin/obfs-local
COPY docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 2260
VOLUME ["/var/cache/resin", "/var/lib/resin", "/var/log/resin"]

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/resin"]
