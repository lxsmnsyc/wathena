#!/bin/sh
# Makes the stack's secrets in ./secrets, next to this file. It keeps
# files that exist, so running it again is safe. To change a password,
# delete its file, run this again, then `docker compose up -d
# --force-recreate`.
#
# Needs openssl. Passwords are 32 random bytes in hex, so they need no
# escaping in database URLs.
set -eu
cd "$(dirname "$0")"
umask 077
mkdir -p secrets
# Only this user may enter the folder. The files inside are readable by
# anyone who can reach them, so the containers' users (postgres, nonroot)
# can read their bind-mounted copies.
chmod 700 secrets

new() {
	if [ ! -s "secrets/$1" ]; then
		printf '%s\n' "$2" >"secrets/$1"
		echo "made secrets/$1"
	fi
}
new postgres_password "$(openssl rand -hex 32)"
new migrator_password "$(openssl rand -hex 32)"
new game_password "$(openssl rand -hex 32)"
new account_password "$(openssl rand -hex 32)"
# The role that verify-backup restores with (decision 0048).
new verify_password "$(openssl rand -hex 32)"
# The key that encrypts TOTP secrets. Back it up apart from the database.
new account_secret_key "$(openssl rand -base64 32)"

# The URLs follow the passwords, so they are written every time.
url() {
	printf 'postgres://%s:%s@postgres:5432/%s?sslmode=disable\n' "$1" "$(cat "secrets/$2")" "${4:-wathena}" >"secrets/$3"
}
url wathena_migrator migrator_password migrate_database_url
url wathena_game game_password game_database_url
url wathena_account account_password account_database_url
# The verify role makes its temporary databases from the postgres
# database. It has no rights on wathena.
url wathena_verify verify_password verify_database_url postgres

# The backup key is not made here, since its identity must stay off the
# server. This only makes the file for its recipient, which the owner
# fills in (deploy/README.md, "Backups").
if [ ! -e secrets/backup_recipients ]; then
	cat >secrets/backup_recipients <<'END'
# The recipient (public key) of the backup key, on a line of its own,
# such as age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p.
# Make the key on your own machine with
# `wactl world backup-key new --out backup-identity.txt` or
# `age-keygen -o backup-identity.txt`, and keep the identity offline.
END
	echo "made secrets/backup_recipients"
fi
if ! grep -q '^age1' secrets/backup_recipients; then
	echo "note: secrets/backup_recipients has no recipient yet, so the backup service fails until you add one"
fi
chmod 644 secrets/*
echo "secrets are in $(pwd)/secrets"
