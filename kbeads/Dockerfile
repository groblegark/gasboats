# Beads daemon (kd) — multi-stage build.
# CGO required for libicu (unicode normalization).
#
# Build:
#   docker build --build-arg VERSION=dev --build-arg COMMIT=$(git rev-parse --short HEAD) -t kd .

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libicu-dev gcc g++ && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Build=${COMMIT}" \
    -o /kd ./cmd/kd

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata libicu74 netcat-openbsd && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd -f -g 1000 beads && \
    useradd -u 1000 -g beads -s /bin/bash -m -d /home/beads beads || true

COPY --from=builder /kd /usr/local/bin/kd

USER 1000
WORKDIR /home/beads

ENTRYPOINT ["kd"]
CMD ["serve"]
