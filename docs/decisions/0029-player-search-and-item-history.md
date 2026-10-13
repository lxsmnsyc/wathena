# 0029: Player Search, Item History, Restores and Audit Reviews

- Status: proposed.
- Date: 2026-10-13.
- Code: [server/admin](../../server/admin) (`players.go`, `world.go`), [server/adminworld](../../server/adminworld), [server/zone](../../server/zone) (`stafftools.go`), [server/store](../../server/store) (`hold.go`, `staffviews.go`, `commit.go`, migration 00024, `queries/admin.sql`, `queries/history.sql`), [server/staff](../../server/staff), [server/mail](../../server/mail) (`expire.go`), [server/chat](../../server/chat) (`Hub.Locate`), [server/transfer](../../server/transfer) (`KindStaff`, `ActionRestore`), [server/cmd/waserver](../../server/cmd/waserver) (`restore.go`), [schema/proto/wathena/admin/v1](../../schema/proto/wathena/admin/v1), [client](../../client) (`src/admin`), [e2e](../../e2e) (`restore.go`).
- Builds proposal 0002, review decision 34, and phase 6 step 2 of strategy section 15. Builds on decisions [0017](0017-e2e-and-transfers.md), [0023](0023-storage-cart-trade.md), [0024](0024-mail-vending-shops.md) and [0028](0028-staff-roles-and-audit.md). It adds no Lua API.

## Decision

### Permissions

- The catalog gains `players.read_ip` and `items.create`. Step 2 uses `audit.review`, `players.read`, `players.read_ip`, `items.history`, `items.restore` and `items.create`.
- The default roles follow review decision 34:
  - Every default role has `players.read`.
  - `admin` and `game_master` have `items.history`.
  - Only `admin` has `players.read_ip`, `items.restore`, `items.create` and `audit.review`. `game_master` loses the `items.restore` that decision 0028 gave it.

### Calls

| Call | Permission | Kind |
|---|---|---|
| `GET /players` | `players.read`, and `players.read_ip` for `ip` | read |
| `GET /characters/{id}` | `players.read` | read |
| `GET /characters/{id}/logs` | `items.history` | read |
| `GET /items/{serial}/history` | `items.history` | read |
| `POST /restore` | `items.restore`, and `items.create` for copies and zeny | change, recent login |
| `GET /audit?needs_review=true` | `audit.read` | read |
| `POST /audit/{id}/review` | `audit.review` | change |

- Every call has step 1's checks: the rate limit, the CSRF token of a change, the session, the role with its two-factor rule, and the permission. Restores also need a login from the last 10 minutes. Reviews do not.
- `GET /players` takes one of `name`, `id`, `serial` or `ip`:
  - `name` is a prefix of an account or a character name, in any case. LIKE's wildcards in it are plain characters.
  - `id` is an account ID or a character ID.
  - `serial` finds the place that holds the serial now, and its account.
  - `ip` is a CIDR range, or one address, matched against the login records of the last 90 days.
  - Pages go by account ID: `limit` is 1 to 50, 20 by default, and `cursor` is the last page's `next_cursor`.
  - Each account shows its characters with their job, levels, online state, position and sanctions in effect, and its last login. An IP search adds the logins that matched, newest first, at most 20 per account.
- `GET /characters/{id}` shows the job, levels, stats, points, HP and SP, zeny, currencies (the account's too), position, save point, party, guild, inventory with worn items, the cart, the account's storage, extra storages, mails with their items, and sanctions in effect.
- `GET /characters/{id}/logs` lists the item and zeny log lines from or to the character's inventory, cart or vending shop, newest first. `since` and `until` bound the times. `limit` is 1 to 100, 50 by default. The cursor holds a time and a row ID for each log, so one page merges both logs.
- `GET /items/{serial}/history` lists the moves oldest first, with where the serial is now.
- Errors keep step 1's codes. A restore whose items keep moving answers `conflict`.

### Where data comes from

- The admin API keeps its account role. It reads accounts, characters, login records and the audit log.
- Game data comes from `admin.World`. In the all-in-one build it is `server/adminworld`, which uses the game's store and the game role:
  - The chat hub says which map an online character is on (`Hub.Locate`). A character that has a session but is not in the hub, such as an autotrader without its connection, is looked for in every zone.
  - An online character is read on its zone's goroutine (`Zone.StaffView`, `Zone.StaffPositions`).
  - An offline character is read from the database without a session (`Store.ReadCharacter`), so its save version does not change.
  - Names of owners in the logs come from the game tables: characters, accounts, guilds and mails.

### Login records

- `account_logins` keeps one row per login of the account pages: the account, the time, the IP address and the user agent (migration 00024). Sessions are deleted sooner, when they expire or go idle, so they cannot serve as the 90-day login records of strategy section 7.10.
- `CreateSession` writes the row in the same statement as the session. The account service's cleanup deletes rows older than 90 days.
- The migration copies the sessions of the last 90 days as the first rows.
- A GiST index serves the `<<=` search. `text_pattern_ops` indexes serve the name prefixes.

### Item history

- The history starts at the serial and follows `source` links:
  - Back: a line whose serial is the serial or an ancestor, with another source, makes the source an ancestor.
  - Forward: a line whose source is the serial or a descendant makes its serial a descendant.
  - It does not follow the ancestors of descendants, or the descendants of ancestors. A merge into a large stack would bring in every other stack that joined it.
- Each round is two queries, `serial = ANY` and `source = ANY`, which each partition of `item_log` answers from `item_log_serial` and `item_log_source`. It stops at 100 serials, 2,000 lines or 20 rounds, and says it was truncated.
- `TestStaffQueryPlans` checks with `EXPLAIN` that every partition uses an index for these queries, the character logs, the name prefixes and the IP range.
- Before a read, the world waits up to 2 seconds for the commits queued so far (`Store.Settle`), so the database shows what the zones did.

### Restores

- The request names a character, serials with an optional amount, optional zeny, and a reason. At most 20 serials, each once.
- For each serial, the world finds the saved place that holds it, after `Settle`:
  - In no place, the serial is gone. The restore makes a copy of its last log line, with the item, refine, cards, rolls, options and bond, and the line's amount unless the request names one. It needs `items.create`.
  - In an inventory or a cart of an online character, the move runs in that character's zone.
  - Anywhere else, the move runs in the start zone, on the shared objects with their locks: storages and carts from the storage registry, mails from the mail registry, guild storages from the social registry.
- The characters that own an offline holder are held in the store (`Store.Hold`):
  - A hold fails while a character has a session that has not ended. The restore then looks for the character's zone, or starts over.
  - A login waits while a hold keeps its character. `Hold.Load` loads an offline inventory for the restore itself, and its session ends with a final save after the commit.
  - Storages and mailboxes of offline holders are acquired from their registries, which load them, and released after the commit is queued.
- The zone checks every serial again, under the locks: guild storages first, then storages, carts and mails. It refuses:
  - a serial that is not in its holder, which starts the restore over,
  - a storage or guild storage that someone has open, which starts it over too,
  - an amount above the stack.
  It starts over up to 8 times, from the database, then answers `conflict`.
- One transfer, `restore`, moves every serial into a new mail from "Staff" to the character, with one slot per serial, so no stack merges and each moved stack keeps its serial. A part of a stack gets a new serial with the stack as its source. Copies and zeny come from the end `staff`, whose ID is the staff account's.
- What the holder sees:
  - A worn item comes off first.
  - An online holder sees the items leave its inventory as taken, or its cart as a cart removal.
  - Its open trade closes.
  - Its vending shop loses the lines of the stack, or closes with no line left.
- The commit holds the holders' rows, the mails, the log lines and the audit row, so one transaction writes them. `CommitAll` now puts everything it gets in one commit under one lock. Before, its rows and its characters could land in two commits.
- The audit row is `items.restore`, marked for review, with the reason, the character, the mail, and each item with its serial, new serial, amount, copy mark and holder. The game role writes it, as the commit's last rows.
- Mail expiry marks a mail it read from its row before it changes it, unless the mailbox loaded meanwhile. A restore that loads the mailbox then works on the live mail, or does not see a mail that expiry took.

### Reviews

- `staff_audit_reviews` gains `verdict`, `ok` or `flag`. A flag needs a note, by a database check.
- A review needs a row marked for review that has no review, by another account than the row's. It writes an `audit.review` row in its transaction.
- `GET /audit` gains `needs_review`. The queue is `needs_review=true&review=pending`. Rows carry the verdict, the note and the staff account's ID.

### Audit rows of lookups

- `players.view` with the character's name and ID for a detail view.
- `players.search_ip` with the range and the number of accounts for an IP search.
- Name, ID and serial searches, logs and histories write none.

### Admin screens

The admin bundle gains tabs, shown by permission:

- Players: the search, the character's details with tabs (Overview, Inventory, Equipment, Storage, Mail, Logs), and serials that open their history.
- Item history: a table of moves oldest first, marked as this serial, came from or went into, and the holder now or "gone".
- Restore: character, serial and amount rows, zeny with `items.create`, and a reason. It shows the mail and each item, or the error with the reauth link.
- Reviews: the queue with a note, ok and flag. A caller's own rows show "Your own action" and no buttons.

### Development endpoint

- With `--dev` and a database, `POST /debug/wathena/restore?serial=&to=&amount=` restores a serial as a dev staff account without `items.create`, for the duplication tests.

## Tests

- `server/staff`: the new names and the default roles.
- `server/admin`: HTTP tests of every call against PostgreSQL with a fake world: searches by name, prefix escapes, pages, ID, serial and IP, the IP search's permission and audit rows, detail views and their audit rows, logs and histories with their queries and errors, restores with the CSRF token, the recent login, `items.create`, bad bodies and the world's error codes, and reviews with self reviews, flags without notes, second reviews, the queue and the review's audit row.
- `server/store`: `TestHold`, `TestSettle`, `TestStaffQueries`, `TestItemHistoryQueries`, `TestStaffQueryPlans` and `TestRestoreAuditInCommit`.
- `server/zone`: restores from an online inventory, during a trade, of a worn item, of copies and zeny, from an offline inventory and an open or closed storage, and from a vending cart.
- `server/adminworld`, with PostgreSQL and a running world: a restore from every kind of place in one mail with its history, logs and audit row, a restore while its holder logs in and out over and over, and the search states.
- `e2e`: `TestDuplicationRestoresWithDatabase` runs the trade duplication test, with storages, carts and connections that close, while restores take random serials from the bots, storages, carts and mails and give them to random bots. The totals, the serials held once and the saved rows hold. Storage and cart moves whose stack a restore took get no answer, as rAthena's, and the move counts that.
- `client`: tests of the query builders, the API calls, the formatting helpers, permission gating and the review buttons. `pnpm check:browsers` loads the player screens with a faked admin API.

## Other choices

- Reading game tables with the account role. It would give the account service, which faces the internet, read access to every inventory and log. The world reads them instead, and the account role keeps its rights.
- Session rows as login records. They are deleted when they expire or go idle, well before 90 days, so an IP search would miss most logins.
- One mail per holder's zone, so a restore from players on several maps could run in one call. It would split the restore into several transactions. The restore asks for separate calls instead.
- Merging restored stacks in the mail, as a sent mail does. The restored stacks would lose their serials, and the history would need a merge to follow them.
- Charging zeny restores to `items.restore`. Zeny has no serial, so a restore always makes it, and making zeny needs `items.create`.

## Left

- The phase 6 exit check runs the admin tools in a browser (step 8).
- Sanctions and GM commands come with step 3. The search shows mutes and jails, and bans come with step 3.
- A network form of `admin.World` for the distributed build (phase 10).
- Restoring items held by players on different maps in one call.
