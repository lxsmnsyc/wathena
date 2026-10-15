# 0028: Staff Roles and the GM Audit Log

- Status: proposed.
- Date: 2026-10-12.
- Code: [server/staff](../../server/staff), [server/admin](../../server/admin), [server/account](../../server/account) (`staff.go`, `tickets.go`, `twofactor.go`, `limit.go`), [server/store](../../server/store) (`staff.go`, migration 00023, `queries/staff.sql`), [server/zone](../../server/zone) (`staffscripts.go`, `headless.go`), [server/modscript](../../server/modscript) (`staff.go`), [server/cmd/waserver](../../server/cmd/waserver), [schema/proto/wathena/admin/v1](../../schema/proto/wathena/admin/v1), [client](../../client) (`admin.html`, `src/admin`), [mods/examples/greeter](../../mods/examples/greeter).
- Builds proposal 0002, review decision 33, and phase 6 step 1 of strategy section 15. Builds on decisions [0004](0004-account-service-and-game-tickets.md), [0005](0005-two-factor-login-and-passkeys.md) and [0026](0026-local-checks.md).

## Decision

### Roles and permissions

- Permission names come from a fixed list in Go, `staff.Catalog`. Each name is `group.name`. A config names a permission, or a whole group as `group.*`. A name that is not in the list, or a group with no names, stops the server from starting.
- Step 1 uses `staff.assign` and `audit.read`. The list also has the names that later steps of phase 6 use, so the default roles and configs can list them now. They do nothing yet:
  - Step 2: `audit.review`, `players.read`, `items.history` and `items.restore`.
  - Step 3: `moderation.reports`, `moderation.mute`, `moderation.kick`, `moderation.jail`, `moderation.ban`, `commands.warp`, `commands.hide` and `commands.item`.
  - Step 4: `world.read`, `world.announce`, `world.restart`, `events.control`, `mods.read` and `mods.disable`.
- `staff.owners` lists account names with the `owner` role. It has every permission and comes only from the config. `owner` cannot be a name in `staff.roles`, and the database refuses it in `staff_roles`.
- `staff.roles` maps role names to permission lists. When it is not set, the roles are the defaults:
  - `admin`: every group, `staff.*`, `audit.*`, `players.*`, `items.*`, `moderation.*`, `commands.*`, `world.*`, `events.*` and `mods.*`.
  - `game_master`: `audit.read`, `players.read`, `items.history`, `items.restore`, `moderation.*`, `commands.*`, `world.read`, `world.announce` and `events.control`.
  - `moderator`: `players.read`, `moderation.reports`, `moderation.mute`, `moderation.kick` and `moderation.jail`.
- When `staff.roles` is set, it replaces the defaults. It does not add to them.
- A role name is 1 to 32 lower-case letters, digits or underscores, starting with a letter.
- Owners and holders of `staff.assign` give the other roles from the admin screens. The database keeps them in `staff_roles`, one role per account. A role the config no longer has counts as no role, and the server logs a warning at start.

### The two-factor rule

- A role counts only when both hold:
  - The session logged in with a second factor: a TOTP code, a recovery code or a passkey. The session's `factors` record this.
  - The account still has TOTP or a passkey on.
- An account with a role that does not count gets `two_factor` from the admin API, so the screens can say what is missing.
- Giving and removing a role also needs a login, or a proof on `/account/reauth`, within the last 10 minutes, the account service's reauth window. The reauth page may now go back to `/admin`.
- In the game, a game ticket records `two_factor` when its session met the rule. The gateway passes it, with the account's name, to the player's session. The game role cannot read sessions or two-factor tables, so the ticket carries the answer.

### The admin API

- JSON at `/admin/api/v1/` on the account service's origin. In the all-in-one build that is the server's public URL.
- Messages are protobuf in `schema/proto/wathena/admin/v1`, in their JSON form with lowerCamelCase names. Go and TypeScript types are generated with the game protocol's.
- Calls:
  - `GET /me`: the account, the role that counts, its permissions, the role the account has even when it does not count, whether the login is recent, the CSRF token and the reauth URL. It needs a session and no permission.
  - `GET /staff` (`staff.assign`): the owners of the config, then the given roles, with who gave each and whether each account has TOTP or a passkey on, and the roles of the config.
  - `POST /staff` (`staff.assign`): gives an account a role, in place of its role. The body is `GiveRoleRequest` with an account name, a role and an optional reason.
  - `DELETE /staff/{account}` (`staff.assign`): takes an account's role away. `?reason=` is optional.
  - `GET /audit` (`audit.read`): audit rows, newest first, filtered by `account`, `action`, `target`, `since`, `until` and `review` (`pending` or `reviewed`), with pages of `limit` rows (1 to 100, 50 by default) before `before_id`.
- Every call checks, in this order: the rate limit, the CSRF token of a change, the session, the role and the permission, and the recent login of a change.
- A change sends the token of `/me` in `X-CSRF-Token`. The token is the account pages' HMAC of the CSRF cookie and the session cookie. `Sec-Fetch-Site` and `Origin` are checked as for the forms.
- Errors are `Error` messages with a code: `login`, `two_factor`, `forbidden`, `reauth`, `csrf`, `rate`, `bad_request`, `not_found`, `conflict` or `unavailable`.
- Calls share a token bucket per address: 60 at once, then 1 a second (`account.Limits.Admin`).
- Who may change what:
  - No one changes their own role.
  - No one changes an owner's role from the API.
  - A caller gives or takes only roles whose permissions it holds all of. An admin with the default roles holds every permission, so admins give and take every role but `owner`.
- The API reaches the world through the `admin.World` interface, `StaffChanged(account, role)`. In the all-in-one build it is the world's `staff.Registry`, in the same process. A distributed build can put a network client there.

### The GM audit log

- `staff_audit` has a row per staff action: the time, the account ID and name, the role, the action, the source (`admin` or `game`), the target and its account ID, the arguments as a JSON object, the reason, and `needs_review`.
- Rows have no foreign keys and keep the account's name, so they outlive accounts. They are kept 2 years. The deletion job comes with phase 8.
- A row with `needs_review` needs a reason, by a database check. Actions that create items or zeny set it, from step 2 on.
- Reviews go in `staff_audit_reviews`, one per row, so a review never changes the row.
- Rights (migration 00023):
  - `wathena_game` adds audit rows and reads `staff_roles`. It cannot read, change or delete audit rows or reviews.
  - `wathena_account` gives and removes roles, adds and reads audit rows, and adds and reads reviews. It cannot change or delete them.
- Step 1's actions are `staff.give`, with the arguments `role` and `previous`, and `staff.remove`, with `role`. Each writes its row in the transaction of the change. Giving the role an account has already changes nothing and writes no row.

### The admin screens

- `/admin` serves `admin.html`, a separate entry of the client build. Its script asks `/me`. Only when `/me` returns a role does it load the admin screens with a dynamic `import()`, a chunk of its own. A player's page and `/admin` without a role never load it.
- The game page does not link to `/admin` or load anything of it.
- Step 1 has the staff list, with a form to give a role and a button to remove one, and the audit log viewer with filters and older pages. A `reauth` error shows a link to the reauth page. Text is set with `textContent` only.

### Lua

- `player:get_staff_role()` returns the role name, `"owner"` for an owner, or nil.
- `player:has_staff_permission(name)` returns whether the role has a permission. An unknown name raises an error that lists the names. `group.*` is not a name here.
- Both need `game.read` and follow the two-factor rule of the player's game ticket. A role given or taken in the admin screens applies at once.
- `wa.testing` gains `world:set_staff_role(player, role)`, provisional. It gives a test player a role of the default roles, as if it logged in with two-factor login.

## Tests

- `server/staff`: the catalog's names, `*` matching, the default roles, the load errors, `Covers` and the registry.
- `server/store`: `TestStaffAuditRights` checks that the game and account roles add audit rows and cannot update, delete or truncate them, that the game role cannot read them or change roles, and the review and owner checks.
- `server/admin`: HTTP tests of every call against PostgreSQL, with TOTP logins: no session, no role, a role without a second factor, a role with TOTP on but a password session, a stale login, a missing or foreign CSRF token, a cross-site change, the rules of who may change what, audit filters and pages, bad bodies, the rate limit, and the ticket's two-factor flag.
- `server/modscript`: `TestStaffCalls` and `TestStaffCallsWithoutHost`.
- `server/zone`: `TestStaffRoleOfPlayers` and `TestJoinKeepsStaffLogin`.
- `server/cmd/waserver`: the staff config and `/admin` serving `admin.html`.
- `wactl mod test`: the greeter's Staff Desk, a staff-only NPC, in [examples/greeter](../../mods/examples/greeter).
- `client`: unit tests of the access check, the API calls and the audit cells. `pnpm check:browsers` loads `/admin` with a faked admin API and checks that a staff account loads the admin bundle and a player does not.

## Other choices

- A database role for the admin API alone, which would read the audit log while the account role only adds rows. The account service already holds the sessions, the API runs in its process, and the admin role would need a fourth database URL. The account role reads the log and cannot change it.
- A role on the game ticket. A role taken away between the ticket and the login would still count until the next login. The world's registry is updated through `admin.World` instead, and the ticket carries only the two-factor answer.
- Counting a recovery code as a second factor. It is TOTP's way back in and is as strong as a code. Leaving it out would lock a staff member out of the tools until they set TOTP up again.

## Left

- Staff actions in the game, such as GM commands, and the audit rows they write, come with step 3. `store.WriteStaffAudit` is ready for them.
- Reviews of audit rows, with `audit.review`, come with step 2.
- The deletion job for rows older than 2 years comes with phase 8.
- A game session keeps the two-factor answer of its ticket. Turning TOTP off during play takes effect at the next login. The admin API checks it on every call.
- A network form of `admin.World` for the distributed build (phase 10).
- `wactl world new --admin` marks its account with the old `accounts.staff` column, which gives no role. The server owner names the account in `staff.owners`. Closed: [decision 0036](0036-phase-6-review-changes.md) drops the column, and the command prints the `staff.owners` lines.
