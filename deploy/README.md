# deploy

Docker Compose files and example configuration. See docs/strategy.md, section 3.9.

- `postgres/roles.sql` makes the `wathena` database and three roles. See server/README.md, "Running with PostgreSQL".

| Role | Used by | Rights |
|---|---|---|
| `wathena_migrator` | `wactl migrate` | Owns the database and the schema. |
| `wathena_game` | The game side of `waserver`, through `WATHENA_DATABASE_URL` | Reads and writes characters and inventories, reads and deletes game tickets. No DDL, and no access to passwords or login sessions. |
| `wathena_account` | The account service, through `WATHENA_ACCOUNT_DATABASE_URL` | Reads and writes passwords, login sessions and game tickets, makes accounts and characters. No DDL, and cannot change or delete characters. |

The migrations grant the table rights, and only to roles that exist when they run. A database made with an older `roles.sql`, before `wathena_account` existed, needs the role made and granted by hand before `wactl migrate up` applies migration 00002:

```text
CREATE ROLE wathena_account LOGIN;
GRANT CONNECT ON DATABASE wathena TO wathena_account;
GRANT USAGE ON SCHEMA public TO wathena_account;
```

Running:

1. Run `roles.sql` once as a PostgreSQL superuser, and set each role's password with `\password` in psql.
2. Apply the migrations with `WATHENA_MIGRATE_DATABASE_URL` set to the migrator's URL: `wactl migrate up`.
3. Start `waserver` with `WATHENA_DATABASE_URL` (game role) and `WATHENA_ACCOUNT_DATABASE_URL` (account role) in the environment, never on the command line.
4. Put it behind a TLS proxy, pass `--account-url https://...` so cookies are `Secure`, list the client origins with `--origins`, and name the proxy with `--trusted-proxies`.
