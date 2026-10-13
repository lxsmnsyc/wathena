# 0030: Moderation and GM Commands

- Status: proposed.
- Date: 2026-10-13.
- Code: [server/admin](../../server/admin) (`moderation.go`), [server/adminworld](../../server/adminworld) (`moderation.go`), [server/zone](../../server/zone) (`gmcommands.go`, `social.go`, `warp.go`, `zone.go`, `mobai.go`, `combat.go`), [server/store](../../server/store) (`moderation.go`, migration 00025, `queries/moderation.sql`), [server/account](../../server/account) (`service.go`, `twofactor.go`, `passkeys.go`), [server/gateway](../../server/gateway), [server/staff](../../server/staff), [server/chat](../../server/chat) (`Hub.Find`), [server/item](../../server/item) (`Table.ByName`), [server/transfer](../../server/transfer) (`ActionCommand`), [server/cmd/waserver](../../server/cmd/waserver) (`sanction.go`), [schema/proto](../../schema/proto), [client](../../client) (`src/admin`, `src/viewer`, `src/net`), [e2e](../../e2e) (`moderation.go`).
- Builds proposal 0002, review decision 35, and phase 6 step 3 of strategy section 15. Builds on decisions [0018](0018-chat-and-moderation.md), [0028](0028-staff-roles-and-audit.md) and [0029](0029-player-search-and-item-history.md). It adds no Lua API.

## Decision

### Permissions

- The catalog gains `commands.recall` and `moderation.ban_permanent`. It loses `commands.item`: review decision 35 puts `@item` under `items.create`.
- Step 3 uses `moderation.reports`, `moderation.mute`, `moderation.jail`, `moderation.kick`, `moderation.ban`, `moderation.ban_permanent`, `commands.warp`, `commands.recall`, `commands.hide` and `items.create`.
- The default roles:
  - `admin` keeps every group, so it has `moderation.ban_permanent`.
  - `game_master` lists `moderation.reports`, `moderation.mute`, `moderation.kick`, `moderation.jail` and `moderation.ban` in place of `moderation.*`, so it keeps its moderation set without `moderation.ban_permanent`. It keeps `commands.*`.
  - `moderator` is unchanged.

### Rows (migration 00025)

- `sanctions` keeps kicks and bans too:
  - A mute, a jail and a kick have a character. A ban has an account and no character.
  - Every row has `account_id`. The migration fills it for the rows that exist.
  - A kick ends when it starts. A permanent ban ends at `'infinity'`.
  - A lift records `lifted_at`, `lifted_by` and `lift_reason`.
- `reports` gains `outcome` (`no_action`, `warned` or `sanctioned`), `note`, `closed_by` and `sanction_id`. Only a sanctioned report links a sanction, by a database check. Reports closed before this step have no outcome.
- `wathena_account` reads the columns of `sanctions` that the ban check needs: `id`, `account_id`, `kind`, `reason`, `ends_at` and `lifted_at`. It cannot change them.
- The game role writes every sanction and report row, with the audit row of each in the same transaction.

### Admin API

| Call | Permission | Kind |
|---|---|---|
| `GET /reports` | `moderation.reports` | read |
| `GET /reports/{id}` | `moderation.reports` | read |
| `POST /reports/{id}/close` | `moderation.reports` | change |
| `POST /sanctions` | the kind's: `moderation.mute`, `.jail`, `.kick` or `.ban` | change; a ban needs a recent login |
| `DELETE /sanctions/{id}?reason=` | the sanction's kind's | change; a ban needs a recent login |
| `GET /sanctions` | any permission of the moderation group | read |

- Every call has the checks of step 1: the rate limit, the CSRF token of a change, the session, the role with its two-factor rule, and the permission.
- `GET /reports` filters by `status` (`open` by default, `closed` or `all`), `target` (a name in any case), `kind` (`player`, `line`, `guild_name` or `guild_emblem`), with pages of `limit` (1 to 100, 50 by default) before `before_id`.
- `GET /reports/{id}` shows:
  - The lines the reporter saw, which the report keeps, marked `seen`, and the reported line, marked `reported`.
  - The target's own lines from 5 minutes before to 5 minutes after the report, from the chat log, while it still has them. These are the chat around the reported line that the reporter did not see, such as whispers.
  - The target's 20 newest earlier reports, by name, and its 20 newest sanctions with its account's bans.
  - The emblem as it was, for an emblem report.
- `POST /reports/{id}/close` takes an outcome and an optional note. `sanctioned` needs a `sanctionId`, which must be a sanction of the reported character or its account. The other outcomes take none. A closed report answers `conflict`. A closed report's lines follow the chat log limit again, as before.
- `POST /sanctions` takes a kind, a target, minutes and a reason:
  - A mute, a jail or a kick names a character. A ban names an account.
  - A mute and a jail last 1 to 525,600 minutes (a year). A kick takes 0.
  - A ban lasts 1 to 5,256,000 minutes (10 years), or 0 for a permanent ban, which needs `moderation.ban_permanent` too.
  - The reason is 1 to 255 characters. The player sees it.
  - No one sanctions their own account.
- `DELETE /sanctions/{id}` lifts a mute, a jail or a ban that is in effect, with a reason. A kick cannot be lifted. No one lifts a sanction of their own account.
- `GET /sanctions` filters by `character` (its sanctions and its account's bans), `account` (every sanction of the account), `kind` and `active=true`, with pages before `before_id`. The active bans are `kind=ban&active=true`.
- `GET /players` and `GET /characters/{id}` show the account's ban in effect.

### How sanctions work

- One path in `server/adminworld` serves the admin API and the GM commands.
- A mute or a jail of an online character starts in its zone, on the zone's goroutine. Then the row and the audit row are written. The zone's next save writes the same row again, by its start, which merges.
- A mute or a jail of an offline character is held in the store (`Store.Hold`), so it cannot log in while the row is written. Its next login loads it. A jail sends it from its saved position, and back there.
- A character that moves between zones, or logs in or out, while a sanction looks for it is looked for again, up to 8 times.
- A kick closes the character's connection with `DISCONNECT_REASON_KICKED` and the reason, and ends its autotrade. A character that is not online answers `not_found`. The kick is a row too, for the history.
- A ban writes its row, then closes the connections of every character of the account with `DISCONNECT_REASON_BANNED`, the reason and the end time. Then:
  - The gateway refuses a game login of the account with the same reason. It checks after it claims the character, so a ban during a login closes that login too.
  - The account service refuses a login once the password is right, after the TOTP code, and after a passkey, and shows the reason and the end time on the login page. A wrong password says nothing of a ban.
  - `POST /account/ticket` answers 403 with `{"error": "banned", "reason", "ends_at", "message"}`. `ends_at` is empty for a permanent ban.
- Lifting writes the row first. A lifted mute or jail of an online character then ends in its zone: the player sees the notice, and a jailed player goes back. An offline character loads without it.
- End times show in UTC: "Your account is banned until 2026-10-14 02:44 UTC. Reason: ...".

### GM commands

| Command | Permission | Does |
|---|---|---|
| `@warp <map> [x y]` | `commands.warp` | Moves the staff member, as `atcommand_mapmove`. A cell off the map or not walkable becomes a random one, with a note. |
| `@jumpto <name>` | `commands.warp` | Moves the staff member to a player on any map. |
| `@recall <name>` | `commands.recall` | Brings a player on any map to the staff member. |
| `@hide` | `commands.hide` | Turns hiding on or off. |
| `@kick <name> <reason>` | `moderation.kick` | Kicks a player. |
| `@mute <name> <minutes> <reason>` | `moderation.mute` | Mutes a player. |
| `@jail <name> <minutes> <reason>` | `moderation.jail` | Jails a player. |
| `@item <item> <amount> <reason>` | `items.create` | Makes items in the staff member's inventory. |

- Every chat line that starts with `@` is a command, in any chat tab. It never goes out as chat, never reaches the chat log, the chat hooks or the chat event.
- A command that does not exist, or that the role does not allow, or a role that does not count for the login, answers "Unknown command.". Answers are system lines.
- A name with spaces goes in double quotes, or the words before the minutes are the name. For `@kick`, the longest run of words that names an online player is the name.
- `@mute`, `@jail` and `@kick` act on online players, through the same path as the admin API. They need the game's database, since a character without one has no ID.
- `@item` takes an item ID or an Aegis name, makes the items from the `staff` end with the log action `command`, and writes an audit row marked for review. An item that does not stack is made at most 100 at a time.
- A dead or jailed staff member cannot warp, and a dead or jailed player cannot be recalled.
- A command that acts on another map runs its part on the zone that holds the player, through the player's session, and the answer comes back the same way. No zone waits for another.
- Audit rows: `commands.warp`, `commands.jumpto`, `commands.recall`, `commands.hide` and `commands.item` with the source `game`, and `moderation.mute`, `moderation.jail` and `moderation.kick` as from the admin API, with the source `game`. The admin API writes `moderation.ban`, `moderation.lift` and `moderation.close_report` too.
- Commands are checked before the mute, so a muted staff member can still use them. rAthena swallows commands only for a mute with `MANNER_NOCOMMAND`.

### @hide

As rAthena's `OPTION_INVISIBLE`:

- Other players do not see the hidden staff member. rAthena sends it as `JT_INVISIBLE`, which no client draws, to staff as well, so other staff do not see it either.
- Monsters do not see or target it (`status_check_visibility`, `status_check_skilluse`), and those that chased it stop. No one targets it with an attack.
- The staff member sees itself and can act.
- It lasts across map changes. It ends at logout.

### Protocol

- `DISCONNECT_REASON_KICKED` (10) is new. `DISCONNECT_REASON_BANNED` now carries the ban's reason and end time.
- Reports of guild names and emblems use `Report.guild_id` and `Report.emblem`, which phase 5 added. No change was needed.
- Events a player has not seen when it changes maps now go out before it leaves, so the answer of a command that moved it arrives.

### Admin screens

- Reports: the queue with filters and older pages, and a report screen with the chat lines (the reported one marked, and the reporter's lines told from the target's), the emblem, the target's earlier reports and sanctions, sanction buttons, a "Sanction and close" form, and a close form for no action and warned.
- The player page has a Sanctions tab: mute, jail and kick for the character, a ban for its account with a permanent box only for `moderation.ban_permanent`, and the sanction history with lift buttons.
- Bans: the active bans with lift buttons.
- The emblem shows from a `blob:` URL, which the client's Content Security Policy allows.

### Game client

- The guild window reports another guild's emblem too ("!E"), next to its name.
- A ticket refused for a ban shows the ban's message. A kick or a ban waits 10 seconds before the page goes back to the character list, so the reason can be read.
- Lines that start with `@` go to local chat from any tab.

### Development endpoints

With `--dev`:

- `POST /debug/wathena/staff?name=&role=` gives an online character's account a role, as if it logged in with a second factor.
- `POST /debug/wathena/sanction?kind=&name=&minutes=&reason=` gives a sanction as `POST /sanctions` does, as a dev staff account. It needs a database.

## Tests

- `server/staff`: the new names and the default roles.
- `server/store`: `TestModerationRows` (sanctions, merges with the zone's save, bans, lifts, lists, the report queue with its lines and closing) and `TestModerationRights`.
- `server/admin`: HTTP tests of every call with a fake world: filters and pages, bad bodies, each kind's permission, the recent login of bans and their lifts, `moderation.ban_permanent`, the CSRF token, the two-factor rule, kicks of players not online, bans in the search, and `TestBannedLogin` for the login page, the TOTP login and the game ticket of a banned account, permanent and lifted bans, and the account role's rights.
- `server/zone`: `TestGMCommandsNeedRoles`, `TestGMWarp`, `TestGMJumpToAndRecall`, `TestGMHide`, `TestGMModerationCommands`, `TestGMItem` and `TestLiftSanctionInZone`.
- `server/adminworld`, with PostgreSQL and a running world: `TestSanctions`, `TestCommandSanctions` and `TestReportQueue`.
- `e2e`: `TestBanKicksAndRefusesLogin`, `TestMuteBlocksChat`, `TestJailMovesPlayer` (with `@jail` and `@kick`) and `TestCommandsNeedPermission` (with `@item`, `@hide`, `@warp` and `@recall`).
- `client`: tests of the query builders, the API calls, the sanction form's checks, permission gating, the guild emblem report, the ticket's ban message, the disconnect texts and the routing of `@` lines. `pnpm check:browsers` opens the report queue, a report, an emblem report, the bans and the sanction tab with a faked admin API.

## Other choices

- A table of bans apart from `sanctions`. One table gives every sanction one ID for `DELETE /sanctions/{id}`, one list per target, and one link from a report.
- The account role writing bans. The world needs to kick the account's characters in the same call, and the game role already writes every other sanction, so the world writes bans too. The account role only reads them.
- A Kicker in the zone for `@kick`. The command takes the admin API's path instead, so a kick from either source writes the same row.
- Showing a hidden staff member to other staff. rAthena does not, and the review asked to follow rAthena.

## Left

- IP bans, and `@` commands from mods (review decision 35).
- `@hide` ends at logout. rAthena saves `OPTION_INVISIBLE` with the character.
- A hidden staff member's local chat does not show to others, since they do not know its entity. rAthena shows it.
- A network form of `admin.World`, and of the kicks, for the distributed build (phase 10).
