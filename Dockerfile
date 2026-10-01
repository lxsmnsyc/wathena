# syntax=docker/dockerfile:1
#
# Two images, built from one Go build stage:
#   - The default target is the all-in-one server: waserver with the web
#     client built into it.
#       docker build -t wathena/waserver:local .
#   - The tools target is wactl on the PostgreSQL 18 image, which has
#     pg_dump and pg_restore. It applies migrations, makes the world,
#     builds assets, and makes and restores backups.
#       docker build --target tools -t wathena/wactl:local .
#
# Rules:
#   - Both images run as user 65532 and work with a read-only root
#     filesystem. /srv/assets belongs to that user, so a new Docker volume
#     mounted there does too.
#   - Neither image has secrets or game assets. deploy/README.md shows how
#     the operator gives both.
#   - Base images are pinned by digest. Update them together with the
#     versions in go.mod and client/package.json.

# Builds the web client into /src/client/dist.
FROM node:26-trixie-slim@sha256:ec7758ee051e457b468b32bde57b0879010b325bb9862718e9615225ce4aaae1 AS client
RUN npm install --global --no-fund --no-audit pnpm@12.7.0
WORKDIR /src/client
COPY client/package.json client/pnpm-lock.yaml client/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.cache/pnpm-store \
    pnpm config set store-dir /root/.cache/pnpm-store && \
    pnpm install --frozen-lockfile
COPY client/ ./
RUN pnpm build

# Builds static Go binaries. Nothing needs cgo.
FROM golang:1.27.1-trixie@sha256:3b77fc618ec235a1ab412de7737f120dd507c57e8d87de4cbb7994fb94275ed5 AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=client /src/client/dist client/dist
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -buildvcs=false -tags webclient \
      -ldflags "-s -w -X main.buildVersion=${VERSION}" \
      -o /out/waserver ./server/cmd/waserver && \
    go build -trimpath -buildvcs=false -ldflags "-s -w" -o /out/wactl ./tools/wactl && \
    mkdir -p /out/srv/assets

# wactl with the PostgreSQL 18 client tools.
FROM postgres:18@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722 AS tools
RUN groupadd --gid 65532 nonroot && \
    useradd --uid 65532 --gid 65532 --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin nonroot
COPY --from=server --chown=65532:65532 /out/srv /srv
COPY --from=server /out/wactl /usr/local/bin/wactl
USER 65532:65532
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/wactl"]
CMD []

# The all-in-one server.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=server --chown=65532:65532 /out/srv /srv
COPY --from=server /out/waserver /usr/local/bin/waserver
COPY deploy/docker/waserver.yml /etc/wathena/waserver.yml
USER 65532:65532
EXPOSE 8300
HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=3 \
  CMD ["/usr/local/bin/waserver", "health"]
ENTRYPOINT ["/usr/local/bin/waserver"]
CMD ["all", "--config", "/etc/wathena/waserver.yml"]
