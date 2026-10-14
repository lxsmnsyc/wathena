# 0032: The Account Page

- Status: proposed.
- Date: 2026-10-14.
- Code: [server/account](../../server/account) (`accountpage.go`, `export.go`, `deletion.go`, `privacy.go`, `limit.go`, `service.go`, templates), [server/adminworld](../../server/adminworld) (`account.go`, `deletion.go`), [server/store](../../server/store) (`accountpage.go`, migration 00027, `queries/accountpage.sql`), [server/social](../../server/social) (`deletion.go`), [server/mail](../../server/mail) (`mail.go`), [server/zone](../../server/zone) (`social.go`), [server/gateway](../../server/gateway), [server/cmd/waserver](../../server/cmd/waserver) (`config.go`, `main.go`, `market.go`, `sanction.go`), [schema/proto](../../schema/proto), [client](../../client) (`src/net`), [e2e](../../e2e) (`account_test.go`).
- Builds proposal 0002, review decision 37, and phase 6 step 5 of strategy section 15. Builds on strategy section 7.10 and decisions [0004](0004-account-service-and-game-tickets.md), [0018](0018-chat-and-moderation.md), [0028](0028-staff-roles-and-audit.md), [0029](0029-player-search-and-item-history.md) and [0030](0030-moderation-and-gm-commands.md). No Lua API changes.

## Decision

### Pages

The account pages gain three pages, linked from a menu on every page of a logged in player:

| Page | What it has |
|---|---|
| `GET /account/safety` | Blocks, reports and sanctions: each character's block list with an Unblock button, the reports the characters filed, and the sanctions on the account and its characters. |
| `GET /account/data` | Your data: the download of the data export, and the account deletion form. While a deletion waits, the date and a Cancel button. |
| `GET /account/privacy` | The privacy notice. It needs no login. |

The changes are form posts:

| Post | Checks |
|---|---|
| `POST /account/safety/unblock` | CSRF token, session, rate limit. |
| `POST /account/data/export` | CSRF token, session, rate limit, a login from the last 10 minutes, once a day. |
| `POST /account/data/delete` | CSRF token, session, rate limit, a login from the last 10 minutes, the account name typed again. |
| `POST /account/data/cancel` | CSRF token, session, rate limit. |

- The CSRF check is the one of every account form: the `Sec-Fetch-Site` or `Origin` header, and the HMAC token of the CSRF and session cookies.
- The rate limit is a new bucket of `Limits`, `Account`: 10 changes at once, then 1 every 6 seconds per address.
- A post without a recent login goes to the reauth page, which comes back to `/account/data`. The player then presses the button again.
- A cancel needs no recent login. The button shows right after a login, and a cancel only keeps the account.
- The characters page shows a waiting deletion's date and the Cancel button at the top, and hides the Play buttons.

### The game side

The account service runs as the account role, which cannot read the game's tables. It asks the game side through `account.Game`. waserver passes `adminworld.World.AccountPage()`:

- Block lists come from the zone of an online character, on the zone's goroutine, and from `character_blocks` for an offline one.
- Unblock works whether the character is online or not. An online character's zone takes the name off and sends the player its new list. An offline character's row is deleted under a store hold, so no login reads the list meanwhile. A character that logs in or out during the call is tried again, as a staff sanction is.
- Reports and sanctions come from the database.
- The export reads offline characters only. It is refused while a character of the account has a session, an autotrader too, with "A character of your account is in the game. Log out every character, and close shops on autotrade, then try again." The refusal does not use up the day's export. It holds the characters while it reads them.
- This is the simpler of the two options of the task. Reading an online character from its zone would need the zone to copy its mailbox, storages and logs at one moment, and the export could still miss a commit on its way.

### Reports and sanctions

- A report shows its date, the character that filed it, its kind and target, the reason, and its status: Open, Action taken or No action.
- A closed report with the outcome `warned` or `sanctioned` shows Action taken. One with `no_action`, or closed before outcomes existed, shows No action. The staff note, who closed it and the linked sanction never show.
- A sanction shows its kind, the character or "The account" for a ban, the reason, the start, the end ("Never" for a permanent ban, "At once" for a kick), and whether it is in effect, ended, or lifted at a time.
- Sanctions are listed active and past, newest first, at most 1,000. Reports too.

### Data export

The export is one JSON file, built at the request and sent as a download named `wathena-<account>-<date>.json`. The account part comes from the account role, the rest from `Game.Export`.

The format is stable. A change that removes or renames a field, or changes its meaning, raises `version`. New fields keep it. Lists are `[]` when empty, never `null`. Times are RFC 3339 in UTC.

| Field | What it holds |
|---|---|
| `format` | `wathena-account-export`. |
| `version` | 1. |
| `exported_at` | When the file was built. |
| `account` | `id`, `name`, `email` (always `null`: the account service stores no email), `created_at`, and `two_factor`, whether TOTP or a passkey is on. |
| `logins` | The login records of the last 90 days, newest first: `at`, `ip`, `user_agent`. |
| `storages` | The account's storages, which its characters share: `kind` (`storage` or `extra`), `storage` (the record ID, 0 for the account's storage), `name`, `items`. |
| `currencies` | The account's currencies: `id`, `name`, `amount`. |
| `characters` | Each character, in slot order. |
| `chat` | The lines the characters said that the chat log still keeps, oldest first: `at`, `character`, `kind`, `target`, `map`, `text`. |
| `item_log` | The item log lines of the characters' inventories, carts, vending shops and own extra storages, and of the account's storages, oldest first: `at`, `action`, `from` and `to` (`kind`, `id`, `storage`), `serial`, `source`, `item_id`, `name`, `amount`, `refine`, `cards`, `bound`. |
| `zeny_log` | The zeny and currency log lines of the same holders: `at`, `action`, `from`, `to`, `amount`, `currency` (0 for zeny). |
| `reports` | The reports the characters filed: `id`, `at`, `character`, `target`, `kind` (`player`, `line`, `guild_name` or `guild_emblem`), `reason`, `status` (`open`, `action_taken` or `no_action`). |
| `sanctions` | The sanctions on the account and its characters: `id`, `kind`, `character` (`""` for a ban), `reason`, `started_at`, `ends_at` (`null` for a permanent ban), `lifted_at` (`null` unless lifted). |
| `truncated` | The names of the logs that reached 100,000 lines, of `chat`, `item_log` and `zeny_log`. Such a log holds its oldest 100,000 lines. |

A character has:

- `id`, `slot`, `name`, `job`, `job_name`, `sex` (`female` or `male`), `base_level`, `job_level`, `base_exp`, `job_exp`.
- `stats` (`str`, `agi`, `vit`, `int`, `dex`, `luk`, the raised stats), `status_points`, `skill_points`, `hp`, `sp`, `zeny`.
- `position` (`map`, `x`, `y`), `save_point` (`null` for the world's start point), `created_at`, `party` and `guild` (names or `""`).
- `skills`: `id` and `level`.
- `inventory`: the items, with the equipment. An item has `serial`, `item_id`, `name`, `amount`, `slot`, `refine`, `cards`, `equip` (the equip location bits where it is worn, or 0), `identified`, `bound` and `rolls` (name to value).
- `cart`, `storages` (the character's own extra storages), `mail` (`id`, `from`, `title`, `body`, `sent_at`, `zeny`, `read`, `returned`, `items`), `currencies` and `block_list`.

What it never holds: the password or its hash, the TOTP secret, recovery codes or their hashes, passkey credentials, public keys or user handles, session or ticket tokens or their hashes, and other accounts' data beyond the names and IDs in the logs and reports. `TestExportHasNoSecrets` checks the file for each of them in text, hex and base64.

The time of the last export is in `account_exports`. An export within 24 hours of the last one is refused with the time the next one is possible. The time is recorded after the file is built, with a statement that changes nothing when another export won the race.

### Account deletion

A request:

- needs a login from the last 10 minutes and the account name typed again, in any case.
- is refused while a character of the account leads a guild with other members: "<character> leads the guild <guild>, which has other members. Make another member the guild's master first."
- is refused for an account that `staff.owners` names. A new account of the same name would become an owner once the name is free.
- is allowed for a banned account.
- writes `account_deletions` with `delete_after` 14 days from now on the database's clock.
- closes the game connections of the account's characters at once, with `DISCONNECT_REASON_ACCOUNT_DELETION` and the date, and ends their autotrade shops.

During the wait:

- `POST /account/ticket` answers 403 `{"error": "deleting", "delete_after": ..., "message": ...}`. The client shows the message.
- `POST /account/play` answers 403 with the same message.
- The gateway refuses a game login of a character of the account, after it claims the character, with `DISCONNECT_REASON_ACCOUNT_DELETION` and the message. This covers a ticket made before the request, and `--dev` logins.
- An autotrade store saved before a restart does not open again.
- The login works. The characters page shows "Your account is set to be deleted on <date>" and a Cancel button. A cancel deletes the request, and play works at once.

The message is "This account is set to be deleted on <date>. Its characters cannot play until then. Cancel the deletion on the account page to play again."

The deletion job runs in waserver on the game role, at start and every hour, on the server clock. For each account whose `delete_after` has passed, at most 100 a run:

1. It holds the account's characters in the store. A character that has a session, an autotrader too, is disconnected with the reason, and the account waits for the next run.
2. The social registry lets the characters go. Each leaves its party, which breaks up when it led it, as the char server does when a leader leaves. Each leaves its guild. A master hands the guild to the member in the lowest slot, as `guildchangegm` does. A master without other members breaks the guild up, and the guild storage's items go with it. The characters go off the friend lists in memory.
3. The mails in memory from the characters lose their sender ID, as their rows do, so they never return to a deleted character.
4. It waits until the store wrote what was queued, then calls `delete_account`.

`delete_account(account, at, account_storages)` is a function of migration 00027 that runs with the rights of the schema's owner. The game role may run it, but cannot delete accounts or characters otherwise. It deletes nothing for an account without a request due at `at`. It runs in one transaction:

| Table | What happens |
|---|---|
| `accounts` | Deleted. The name is free again. |
| `account_passwords` | Deleted with the account. |
| `account_totp`, `account_recovery_codes` | Deleted with the account. |
| `account_webauthn_users`, `account_passkeys` | Deleted with the account. |
| `account_sessions`, `account_pending_logins`, `account_webauthn_ceremonies`, `game_tickets` | Deleted with the account. The player is logged out. |
| `account_logins` | Deleted with the account, IP addresses too. |
| `account_exports`, `account_deletions` | Deleted with the account. |
| `staff_roles` | The account's role is deleted. A role it gave keeps the role with `given_by` NULL. |
| `staff_audit` | Kept for its 2 years. Rows keep `account_id`, `account_name`, `target` and `target_account_id`, for the account as a staff member and as a target. |
| `staff_audit_reviews` | Kept, with the reviewer's ID and name. |
| `event_overrides`, `mod_switches` | Kept, with the staff member's ID and name. |
| `characters` | Deleted. |
| `inventory`, `character_skills`, `character_statuses`, `character_timed_bonuses` | Deleted with the characters. |
| `mod_character_variables`, `mod_account_variables` | Deleted with the characters and the account. |
| `mod_server_variables` | Kept. The engine cannot tell whether a mod keeps a character ID in a value. |
| `storage_items` | The account's storages and the characters' carts and storages are deleted. |
| `storage_sizes` | Deleted. |
| `character_currencies`, `account_currencies` | Deleted. |
| `character_blocks` | The characters' own lists are deleted. Other characters' lists keep the names they block. |
| `character_friends` | Both sides of each friendship are deleted. |
| `parties`, `party_members` | The memberships are deleted. A party the account led alone is deleted. The server broke up the parties it led with others in memory first. |
| `guilds`, `guild_members`, `guild_storage`, `guild_relations` | The memberships are deleted. A guild the account led alone is deleted with its storage and relations. A guild with other members got a new master in memory first. |
| `guilds.expulsions` | Kept in other guilds' rows, with the character's ID and name. |
| `guild_emblems` | Kept. An emblem the account uploaded names account 0. |
| `mails`, `mail_items` | Mails to the characters are deleted with their items. Mails from them stay with the sender's name and a NULL sender ID. Such a mail never returns, and its expiry deletes it. |
| `stores`, `store_items` | Deleted, autotrade stores too. |
| `sanctions` | Deleted with the account and the characters. The staff audit log keeps a row for each. A report that linked one keeps NULL. |
| `chat_log` | The characters' own lines are deleted. Lines of other players stay, whispers to the characters too. |
| `reports` | Kept, with `reporter_id`, `reporter_name`, `target_id` and `target_name`, also for reports by and against the characters. Reports have no account fields. |
| `report_lines` | The links to the deleted chat lines go. A report of a deleted line keeps its row without the line. |
| `item_log`, `zeny_log` | Kept, with the character IDs. Lines of the account's storage, and of its account-owned extra storages, name storage 0 in place of the account. |
| `world`, `mod_ids`, `shop_stock`, `world_event_runs`, `world_event_progress`, `goose_db_version` | Not linked to accounts or characters. |

The character IDs that logs and reports keep belong to no account once the account is gone. A new character never takes an old ID.

### Privacy notice

`/account/privacy` shows, as plain text:

- The owner's text, `privacy.text`, with paragraphs at blank lines.
- The owner's contact details, `privacy.contact`, or a line that there are none.
- What the server keeps and for how long, from the config: login records 90 days, sessions up to their lifetime and idle time, chat lines `chat.log_days`, item and zeny logs 180 days, the staff audit log 2 years. The page says that the deletion jobs of the logs and the audit log come in a later version, so they are kept until then.
- The player's rights: the export, the deletion and its 14 days, and the safety page.

`privacy.contact` holds at most 1,000 bytes and `privacy.text` 20,000.

### Protocol

`DisconnectReason` gains `DISCONNECT_REASON_ACCOUNT_DELETION = 12`. The client shows its message, or "This account is set to be deleted. Cancel the deletion on the account page to play." without one, and waits 10 seconds before it leaves for the character list, as for a ban.

### Database

Migration 00027:

- `account_exports (account_id, exported_at)` and `account_deletions (account_id, requested_at, delete_after)`, both deleted with their account.
- `delete_account`, above.
- Rights: the account role reads, adds and changes exports, and reads, adds and deletes requests. The game role reads requests and runs `delete_account`. Neither can delete accounts or characters with a statement of its own. `TestAccountPageRights` checks each.

### Development endpoints

With `--dev` and a database, `POST /debug/wathena/deletions` runs the deletion job at once on the server clock, and answers `{"deleted": n}`. The end-to-end tests move the clock with `POST /debug/wathena/clock` first.

## Tests

- `server/account`, with PostgreSQL and a fake game side: `TestAccountPagesNeedALogin`, `TestAccountPostsNeedCSRF`, `TestSafetyPage`, `TestAccountPageRateLimit`, `TestExport` (the reauth, a character online, once a day, a day later), `TestExportHasNoSecrets`, `TestDeletion` (the reauth, the wrong name, the guild master, the ticket and Play refusals, the date and Cancel after a new login, the cancel), `TestBannedAccountMayAskForDeletion`, `TestOwnerCannotAskForDeletion`, `TestPrivacyPage` and `TestAccountPagesWithoutGame`.
- `server/store`: `TestDeleteAccountEveryTable` fills every table that names an account or a character for two accounts, runs the deletion as the game role, and checks what goes and what stays. `TestDeletionOfCharacter`, `TestUnblockOffline` and `TestAccountPageRights`.
- `server/social`: `TestForgetCharacters` for parties, guild masters with and without members, and friend lists.
- `server/adminworld`, with PostgreSQL and a running world: `TestAccountPageBlocks` (online and offline unblocks), `TestAccountPageReportsAndSanctions`, `TestAccountPageExport` and `TestDeletionJob` (an online character waits and is disconnected, then the job deletes the account after the clock passes the date, and the party it led breaks up).
- `e2e`: `TestDeletionBlocksPlayUntilCancelled` and `TestDeletionRunsAfterTheWait`, the full run with PostgreSQL after the server clock moved 15 days.
- `server/cmd/waserver`: `TestPrivacyConfig` for the privacy keys and their limits.
- `client`: the ticket refusal and the new disconnect reason.

## Other choices

- Reading online characters for the export from their zones. See "The game side".
- Deleting the account on the account role. The deletion needs the world's memory: parties, guilds, friend lists, mails and autotraders. The job runs where the world is, on the game role, through one function with narrow rights.
- Running the job's deletes as separate statements of the game role. That role would need the right to delete accounts, characters and the account service's tables at any time. The function gives it one call that only works for a due request.
- Checking the due time against the database's clock in the function. A compromised game role could not then delete early. The server clock decides instead, so the tests can move it. A compromised game role could already change any character.
- Keeping sanctions after the deletion, without the account. The staff audit log keeps a row of each sanction with its target for 2 years, which is the staff's record.
- Keeping a deleted account's name reserved. Only owner names matter for the staff roles, and those cannot be deleted while the config names them.
- A new disconnect reason against reusing `DISCONNECT_REASON_BANNED`. The player is not banned, and a ban's line in the client would mislead.

## Left

- The account service stores no email, so the export's `email` is always `null` (review item).
- Mod variables are not in the export. Review decision 37 does not list them (review item).
- A player who sends mail to a character in the second its account is deleted can see the send fail.
- The guild storage items of a guild that the account led alone go with the guild, without item log lines.
- A network form of `account.Game` and the job for the distributed build (phase 10).
- The retention jobs of the item and zeny logs and of the staff audit log (phase 8).
