# deploy

Docker Compose files and example configuration. See docs/strategy.md, section 3.9.

- `postgres/roles.sql` makes the `wathena` database and three roles. See server/README.md, "Running with PostgreSQL".

| Role | Used by | Rights |
|---|---|---|
| `wathena_migrator` | `wactl migrate` | Owns the database and the schema. |
| `wathena_game` | The game side of `waserver`, through `WATHENA_DATABASE_URL` | Reads and writes characters and inventories, reads and deletes game tickets. No DDL, and no access to passwords or login sessions. |
| `wathena_account` | The account service, through `WATHENA_ACCOUNT_DATABASE_URL` | Reads and writes passwords, login sessions, game tickets, TOTP secrets, recovery codes and passkeys, and makes accounts and characters. No DDL, and cannot change or delete characters. |

The migrations grant the table rights, and only to roles that exist when they run. A database made with an older `roles.sql`, before `wathena_account` existed, needs the role made and granted by hand before `wactl migrate up` applies migration 00002:

```text
CREATE ROLE wathena_account LOGIN;
GRANT CONNECT ON DATABASE wathena TO wathena_account;
GRANT USAGE ON SCHEMA public TO wathena_account;
```

Running:

1. Run `roles.sql` once as a PostgreSQL superuser, and set each role's password with `\password` in psql.
2. Apply the migrations with `WATHENA_MIGRATE_DATABASE_URL` set to the migrator's URL: `wactl migrate up`.
3. Make the key that encrypts TOTP secrets, once, and keep it with the other secrets: `openssl rand -base64 32`.
4. Start `waserver` with these in the environment, never on the command line:
   - `WATHENA_DATABASE_URL`: the game role's URL.
   - `WATHENA_ACCOUNT_DATABASE_URL`: the account role's URL.
   - `WATHENA_ACCOUNT_SECRET_KEY`: the key from step 3. Without it, players cannot turn on two-factor codes.
5. Put it behind a TLS proxy, pass `--account-url https://...` so cookies are `Secure`, list the client origins with `--origins`, and name the proxy with `--trusted-proxies`. Passkeys work only on the domain name in `--account-url`.

Environment:

| Variable | Use |
|---|---|
| `WATHENA_DATABASE_URL` | The game role's database URL. |
| `WATHENA_ACCOUNT_DATABASE_URL` | The account role's database URL. |
| `WATHENA_ACCOUNT_SECRET_KEY` | 32 random bytes in base64. Encrypts TOTP secrets in the database with AES-256-GCM. |
| `WATHENA_ACCOUNT_OLD_SECRET_KEYS` | Older keys, comma-separated, after a key change. They only open secrets sealed before the change. |

Changing the key:

1. Move the current key to `WATHENA_ACCOUNT_OLD_SECRET_KEYS`, and set a new `WATHENA_ACCOUNT_SECRET_KEY`.
2. Each secret is sealed again under the new key when its player next logs in with a code.
3. Keep the old key while any row still uses it. A secret whose key is lost cannot be opened, and its player must use a recovery code or a passkey and set TOTP up again.

Back up the key apart from the database. A database backup without the key does not reveal TOTP secrets.
