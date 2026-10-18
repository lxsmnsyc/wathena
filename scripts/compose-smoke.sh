#!/usr/bin/env bash
# Starts the Compose stack from deploy/compose with the placeholder pack,
# as deploy/README.md describes, and checks it through Caddy:
#   1. Makes the secrets, builds the images, and builds the placeholder
#      assets into the assets volume with the offline service.
#   2. Makes a backup key, makes a procedural world and its staff
#      account, then starts the stack without a backup recipient and
#      waits until every service is healthy. The backup service waits
#      and warns.
#   3. Fetches the client, the account login page and the asset manifest
#      over HTTPS from Caddy's local authority, and checks that the
#      health checks are not public. Checks that Caddy is not root, that
#      PostgreSQL has no capabilities, and that the backup service has
#      no migrator secret.
#   4. Gives the backup service its recipient and starts it again,
#      waits for its first encrypted backup, verifies it with the
#      backup-verify service, restores it into an empty database, and
#      starts again (decision 0048).
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
# The test's backup key, so the secrets of a local stack stay as they are.
export WATHENA_BACKUP_RECIPIENTS="$work/backup_recipients" WATHENA_BACKUP_IDENTITY="$work/backup_identity"
# The stack runs the default waserver.yml. Its maps are in the classic
# block, which the procedural world ignores (decision 0049).
export WATHENA_CONFIG=./waserver.yml
compose() { docker compose --env-file .env.example "$@"; }
cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		compose logs --no-color --tail 50 || true
	fi
	compose --profile tools --profile verify down -v --remove-orphans || true
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

./secrets.sh
compose --profile tools build
compose run --rm offline assets build --source placeholder --out /srv/assets
# The key is made in a container, whose /tmp goes with it. Only its
# recipient goes to the backup service.
compose run --rm -T --entrypoint sh offline -c \
	'wactl world backup-key new --out /tmp/identity.txt >/dev/null 2>&1 && cat /tmp/identity.txt' >"$WATHENA_BACKUP_IDENTITY"
# The stack starts first without a recipient, as a new stack does.
echo '# no recipient yet' >"$WATHENA_BACKUP_RECIPIENTS"
# The containers' user must read the bind-mounted copies, as secrets.sh
# does for its files.
chmod 644 "$WATHENA_BACKUP_IDENTITY" "$WATHENA_BACKUP_RECIPIENTS"
openssl rand -hex 16 >"$work/admin_password"
# The release's default mode is procedural, from the reference data
# built into wactl (decision 0047).
compose run --rm -T wactl world new --name Smoke --admin Admin --admin-password-file - <"$work/admin_password" >"$work/world.txt"
grep -q 'made the procedural' "$work/world.txt"
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

# The hardening of the services (security review L18 and L27).
if [ "$(compose exec -T caddy id -u)" != 65532 ]; then
	echo "caddy runs as root" >&2
	exit 1
fi
if ! compose exec -T postgres grep -q '^CapEff:[[:space:]]*0*$' /proc/1/status; then
	echo "postgres has capabilities" >&2
	exit 1
fi
if compose exec -T backup ls /run/secrets | grep -q migrate; then
	echo "the backup service has the migrator's URL" >&2
	exit 1
fi

# Without a recipient, the backup service runs, warns, and makes nothing.
for i in $(seq 30); do
	compose logs --no-color backup >"$work/backup.log"
	if grep -q 'msg="backups are off' "$work/backup.log"; then
		break
	fi
	if [ "$i" = 30 ]; then
		echo "no warning from the backup service without a recipient" >&2
		cat "$work/backup.log" >&2
		exit 1
	fi
	sleep 1
done
if compose exec -T backup ls /backups | grep -q '^world-'; then
	echo "a backup without a recipient" >&2
	exit 1
fi
# The service reads the recipient when it starts.
sed -n 's/^# public key: //p' "$WATHENA_BACKUP_IDENTITY" >"$WATHENA_BACKUP_RECIPIENTS"
grep -q '^age1' "$WATHENA_BACKUP_RECIPIENTS"
compose up -d --force-recreate --wait --wait-timeout 300 backup

# The backup service makes its first backup at start.
for i in $(seq 120); do
	compose logs --no-color backup >"$work/backup.log"
	if grep -q 'msg="backup made"' "$work/backup.log"; then
		break
	fi
	if [ "$i" = 120 ]; then
		echo "no backup after 2 minutes" >&2
		cat "$work/backup.log" >&2
		exit 1
	fi
	sleep 1
done
compose exec -T backup ls /backups >"$work/backups.txt"
backup=$(grep '^world-.*\.dump\.age$' "$work/backups.txt" | tail -1)
grep -qx "${backup%.dump.age}.json" "$work/backups.txt"
compose exec -T backup cat "/backups/$backup" >"$work/world.dump.age"
if [ "$(head -c 22 "$work/world.dump.age")" != "age-encryption.org/v1" ]; then
	echo "$backup is not an age file" >&2
	exit 1
fi
# The verify role exists only once the verify profile runs.
roles() { compose exec -T postgres psql -X -U postgres -tAc "SELECT count(*) FROM pg_roles WHERE rolname = 'wathena_verify'"; }
if [ "$(roles)" != 0 ]; then
	echo "the verify role exists without the verify profile" >&2
	exit 1
fi
compose run --rm -T backup-verify world verify-backup --dir=/backups --identity=/run/secrets/backup_identity
if [ "$(roles)" != 1 ]; then
	echo "the verify profile made no verify role" >&2
	exit 1
fi
compose down
docker volume rm "${COMPOSE_PROJECT_NAME}_pgdata"
compose run --rm -T -v "$WATHENA_BACKUP_IDENTITY:/keys/identity.txt:ro" wactl \
	world restore --in - --identity /keys/identity.txt <"$work/world.dump.age"
compose up -d --wait --wait-timeout 300
check
# grep -q stops reading at the first match, which makes compose logs fail
# on a closed pipe under pipefail. Read the logs from a file instead.
compose logs --no-color waserver >"$work/waserver.log"
grep -q 'world=Smoke' "$work/waserver.log"
echo "compose smoke test passed"
