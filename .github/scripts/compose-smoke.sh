#!/usr/bin/env bash
# Starts the Compose stack from deploy/compose with the placeholder pack,
# as deploy/README.md describes, and checks it through Caddy:
#   1. Makes the secrets, builds the images, and builds the placeholder
#      assets into the assets volume with the wactl service.
#   2. Makes the world and its staff account, then starts the stack and
#      waits until every service is healthy.
#   3. Fetches the client, the account login page and the asset manifest
#      over HTTPS from Caddy's local authority, and checks that the
#      health checks are not public.
#   4. Makes a backup, restores it into an empty database, and starts
#      again.
# It removes the stack and its volumes at the end. It uses the project
# name wathena-smoke and ports 18080 and 18443, so a local stack named
# wathena keeps running.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)/deploy/compose"

# The folder is in the repository, which Docker Desktop shares with its
# VM. A system temporary folder may not be shared.
work=$(mktemp -d "$PWD/smoke.XXXXXX")
export COMPOSE_PROJECT_NAME=wathena-smoke
export WATHENA_DOMAIN=localhost WATHENA_HTTP_PORT=18080 WATHENA_HTTPS_PORT=18443
export WATHENA_EDGE_SUBNET=${WATHENA_EDGE_SUBNET:-172.30.81.0/24} WATHENA_PROXY_IP=${WATHENA_PROXY_IP:-172.30.81.10}
export WATHENA_CONFIG="$work/waserver.yml"
cat >"$WATHENA_CONFIG" <<'EOF'
listen: 0.0.0.0:8300
assets: /srv/assets
start_map: placeholder/map
maps: all
mode: none
EOF
chmod 644 "$WATHENA_CONFIG"
compose() { docker compose --env-file .env.example "$@"; }
cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		compose logs --no-color --tail 50 || true
	fi
	compose --profile tools down -v --remove-orphans || true
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

./secrets.sh
compose --profile tools build
compose run --rm --no-deps wactl assets build --source placeholder --out /srv/assets
openssl rand -hex 16 >"$work/admin_password"
compose run --rm -T wactl world new --name Smoke --admin Admin --admin-password-file - <"$work/admin_password"
compose up -d --wait --wait-timeout 300

check() {
	compose cp caddy:/data/caddy/pki/authorities/local/root.crt "$work/root.crt" >/dev/null
	base=https://localhost:$WATHENA_HTTPS_PORT
	for want in "200 /" "200 /account/login" "200 /assets/manifest.json" "404 /healthz" "404 /readyz"; do
		code=${want%% *}
		path=${want#* }
		got=$(curl -sS --cacert "$work/root.crt" -o /dev/null -w '%{http_code}' "$base$path")
		if [ "$got" != "$code" ]; then
			echo "$path: status $got, want $code" >&2
			exit 1
		fi
		echo "$path: $got"
	done
}
check

(umask 077 && compose run --rm -T wactl world backup --out - >"$work/world.dump")
compose down
docker volume rm "${COMPOSE_PROJECT_NAME}_pgdata"
compose run --rm -T wactl world restore --in - <"$work/world.dump"
compose up -d --wait --wait-timeout 300
check
compose logs --no-color waserver | grep -q 'world=Smoke'
echo "compose smoke test passed"
