# syntax=docker/dockerfile:1.7

# ── Linochi ERP backend: production image (NFK-01, NFK-04) ────────────────
# One static binary on distroless. It carries the CA bundle (system roots only,
# NFK-12: the internal CA is added at run time with SSL_CERT_DIR, see the chart)
# and no shell, so `kubectl exec` runs the binary itself: /api sessions revoke-user.
#
# Built natively on arm64 by the runner (and on amd64 locally). No cross build.

# The Go release of go.mod (and of the nars-ci image). Bump them together.
ARG GO_VERSION=1.27.3

# ── Stage 1: build ────────────────────────────────────────────────────────
FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

# On their own so that this layer is rebuilt only when the dependencies move.
COPY go.mod go.sum ./
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go mod download

COPY . .

# What `api version` prints, what the logs say at start and what
# GET /api/v1/version answers (INS-07). Declared after the copy of the code and
# only here: it changes on every build, and nothing above depends on it.
ARG VERSION=dev
ARG COMMIT=unknown
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/api ./cmd/api

# ── Stage 2: runtime ──────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/api /api

# The numeric id, so that the chart's runAsNonRoot can be checked without
# looking up a name in an image that has no /etc/passwd tools.
USER 65532:65532

# 8080 is the public API (Traefik routes /api and /auth to it), 9090 is
# /healthz, /readyz and /metrics and has no route. No HEALTHCHECK: there is no
# shell or curl to run one with, and Kubernetes uses its own probes.
EXPOSE 8080 9090

ENTRYPOINT ["/api"]
