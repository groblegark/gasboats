# gitlab-bridge: standalone beads→GitLab MR merge sync bridge.
# Multi-stage build: Go builder → distroless runtime.
#
# Build:
#   docker build -t gasboat/gitlab-bridge:latest -f images/gitlab-bridge/Dockerfile \
#     --build-arg VERSION=$(git describe --tags --always) .

FROM golang:1.25-bookworm AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /build
COPY controller/ ./

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /gitlab-bridge ./cmd/gitlab-bridge/

# ── Runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /gitlab-bridge /gitlab-bridge

USER nonroot:nonroot
EXPOSE 8092

ENTRYPOINT ["/gitlab-bridge"]
