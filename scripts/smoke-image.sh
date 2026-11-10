#!/usr/bin/env bash
# Smoke test of the image (NFK-01, NFK-02, NFK-12): runs it the way the chart does
# against a real Postgres and checks what the cluster will depend on.
#
#   scripts/smoke-image.sh <image> <version> <commit>
#
# Everything sits on a network made for the run, and the probes run from inside it.
# `-p 8080:8080` plus `curl 127.0.0.1` would not work under Docker-in-Docker (the
# port is published in the daemon's network namespace, not the step's), and a
# bind mount would not either. So the initdb files go in with `docker cp`, and
# the prober is the Postgres container, because the image under test is distroless
# (no shell, no curl) and bash has /dev/tcp.
set -euo pipefail

image=${1:?usage: smoke-image.sh <image> <version> <commit>}
want_version=${2:?}
want_commit=${3:?}

here=$(cd "$(dirname "$0")/.." && pwd)
# Keep in step with compose.yaml and the cluster (see compose.yaml).
pg_image=${PG_IMAGE:-postgres:18.6-trixie}
net=linochi-smoke-net
pg=linochi-smoke-pg
app=linochi-smoke-app
db="linochi_erp"
host=$pg

cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "── smoke test failed: logs of the app ──"
    docker logs "$app" 2>&1 | tail -40 || true
  fi
  docker rm -f "$app" "$pg" >/dev/null 2>&1 || true
  docker network rm "$net" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT
docker rm -f "$app" "$pg" >/dev/null 2>&1 || true
docker network rm "$net" >/dev/null 2>&1 || true

fail() { echo "SMOKE FAIL: $*" >&2; exit 1; }

# 1. The binary knows what it is, and runs as a number that is not root.
out=$(docker run --rm "$image" version)
[ "$out" = "${want_version} · ${want_commit}" ] || fail "version prints '$out', want '${want_version} · ${want_commit}'"
user=$(docker inspect --format '{{.Config.User}}' "$image")
[ "$user" = "65532:65532" ] || fail "image user is '$user', want 65532:65532"

# 2. A Postgres with the same three roles the cluster has (H1).
docker network create "$net" >/dev/null
docker create --name "$pg" --network "$net" -e POSTGRES_PASSWORD=dev "$pg_image" >/dev/null
docker cp "$here/db/dev/initdb/." "$pg:/docker-entrypoint-initdb.d/"
docker start "$pg" >/dev/null
# -h 127.0.0.1: the temporary server of the first start listens on the socket
# only, so answering over TCP means the real server is up and the roles exist.
for _ in $(seq 1 60); do
  docker exec "$pg" pg_isready -q -h 127.0.0.1 -U postgres -d "$db" && break
  sleep 1
done
docker exec "$pg" pg_isready -q -h 127.0.0.1 -U postgres -d "$db" || fail "postgres never came up"

owner="postgres://linochi_erp_owner:dev@${host}:5432/${db}?sslmode=disable"
appurl="postgres://linochi_erp_app:dev@${host}:5432/${db}?sslmode=disable"

# 3. The migration Job: `api migrate up` as the owner, and nothing but that URL.
docker run --rm --network "$net" -e "MIGRATE_DATABASE_URL=${owner}" "$image" migrate up >/dev/null \
  || fail "api migrate up failed"

# 4. The Deployment: the app role and the login settings, no migrate URL.
#    Authelia does not exist here, and the app must start anyway (NFK-10).
docker run -d --name "$app" --network "$net" \
  -e "DATABASE_URL=${appurl}" \
  -e OIDC_ISSUER=https://auth.smoke.invalid -e OIDC_CLIENT_ID=linochi-erp \
  -e OIDC_CLIENT_SECRET=smoke-only -e OIDC_REDIRECT_URL=https://erp.smoke.invalid/auth/callback \
  -e SHUTDOWN_DELAY=0s \
  "$image" >/dev/null

# get <port> <path>: the status line and the body, from inside the network.
get() {
  docker exec "$pg" bash -c '
    exec 3<>/dev/tcp/'"$app"'/'"$1"' || exit 1
    printf "GET %s HTTP/1.0\r\nHost: smoke\r\n\r\n" "'"$2"'" >&3
    cat <&3'
}

ready=""
for _ in $(seq 1 30); do
  if get 9090 /readyz 2>/dev/null | head -1 | grep -q " 200 "; then ready=1; break; fi
  sleep 1
done
[ -n "$ready" ] || fail "/readyz never answered 200"

get 9090 /healthz | head -1 | grep -q " 200 " || fail "/healthz is not 200"
get 9090 /metrics | grep -q '^linochi_auth_logins_total{result="ok"} 0' || fail "/metrics has no linochi_auth_logins_total"
# The public port carries /api and /auth and not the internal paths.
get 8080 /api/v1/version | head -1 | grep -q " 401 " || fail "/api/v1/version without a session is not 401"
get 8080 /metrics | head -1 | grep -q " 404 " || fail "/metrics is served on the public port"
get 9090 /api/v1/me | head -1 | grep -q " 404 " || fail "/api/v1/me is served on the internal port"
# Authelia is down: the login ends at the error page and does not hang or crash.
login=$(get 8080 /auth/login | tr -d '\r')
echo "$login" | head -1 | grep -q " 303 " || fail "/auth/login did not answer 303"
echo "$login" | grep -qi '^location: /login-error?code=failed' || fail "/auth/login did not go to /login-error?code=failed"

# The operator command from the README, as `kubectl exec` runs it: the binary
# itself in a pod, no shell, as the app role. There is no such user here, so it
# must reach the database and say so: "no such user" is the proof that it logged
# in as the app role, found the tables and ended nothing.
if out=$(docker run --rm --network "$net" -e "DATABASE_URL=${appurl}" "$image" sessions revoke-user nobody@example.invalid 2>&1); then
  fail "revoke-user of an unknown user succeeded: $out"
fi
echo "$out" | grep -q "no such user" || fail "revoke-user did not reach the database: $out"

echo "Smoke test passed: ${image}"
