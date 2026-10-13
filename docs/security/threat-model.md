# Threat Model

This document lists what wAthena protects, who attacks it, and how each threat is handled. It extends strategy section 7.1. Update it when a design proposal changes a trust boundary, and review it in full before the public beta.

Status values:

- **Done.** Built and tested.
- **Decided.** Chosen in the strategy, a decision, or a proposal, but not built yet.
- **Open.** Not decided yet.

## Assets

| Asset | Why it matters |
|---|---|
| Account credentials and session tokens | Control of an account means control of its items and characters. |
| Items and zeny | The economy. Duplication or theft ruins it for everyone. |
| The world secret and secret seed branches | They decide spawns, loot, and quest answers in procedural worlds. |
| Player private data | Email, IP addresses, chat logs, and reports. |
| Server availability | Players cannot play during an outage, and rollbacks lose progress. |
| Staff powers | GM commands can create items, read private data, and ban players. |
| The server host | Code running on it can reach the database and every secret. |
| Build and release artifacts | A tampered release reaches every server that installs it. |

## Trust boundaries

```mermaid
flowchart LR
  subgraph Untrusted
    B[Browser: web client or a fork]
    MI[Community mod indexes]
    CI2[Third-party mod code]
  end
  subgraph Server host
    GW[Gateway]
    AS[Account service and login page]
    ZS[Zone service]
    LUA[C Lua states in the zone process]
    WS[World service]
  end
  DB[(PostgreSQL)]
  B -->|WebSocket over TLS| GW
  B -->|HTTPS| AS
  GW --> ZS
  ZS --> LUA
  CI2 -->|installed by the owner| LUA
  MI -->|metadata only| CI2
  ZS -->|mTLS gRPC| AS
  ZS -->|mTLS gRPC| WS
  AS --> DB
  WS --> DB
  ZS --> DB
```

The boundaries are:

1. **Browser to gateway and account service.** Everything the client sends is untrusted. The client is open source and easy to modify.
2. **Mod code to the zone.** Mod scripts are untrusted code that runs inside the zone process, in C Lua states.
3. **Services to each other.** Services authenticate each other with mutual TLS. A compromised service must not gain more database rights than its own role.
4. **Staff to the admin API.** Staff are trusted with their role, but every action is logged. The admin API runs on the account service's origin and pool, and reaches the world through one interface, `admin.World` ([decision 0028](../decisions/0028-staff-roles-and-audit.md)).
5. **Build to release.** Third-party code and tools enter through Go modules, GitHub Actions, and downloaded compilers.

## Attackers

| Attacker | Goal | Main entry points |
|---|---|---|
| Cheating player | Items, zeny, speed, or information they should not have. | Modified client, bots, injected or replayed messages, predicting random outcomes. |
| Account attacker | Take over accounts to steal items. | Reused passwords, brute force, phishing, stolen session tokens. |
| Malicious mod author | Steal data, add backdoors, break the economy, or take over the host. | Published mods, hooks, permissions, bugs in the Lua runtime. |
| Malicious client fork | Steal passwords or sessions. | Forked web clients hosted by third parties. |
| Rogue staff member | Give items or read private data. | GM commands, the admin tools, database access. |
| Network attacker | Take servers offline, or read traffic. | Connection floods, large or expensive messages, traffic analysis. |
| Web attacker | Run code in players' browsers or act as them. | Chat, names, guild emblems, cross-site requests. |
| Supply chain attacker | Put code into wAthena releases or dependencies. | Go modules, Actions, compilers, the Lua source. |

## Threats and mitigations

### Client and protocol

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| Crafted messages crash the server. | Generated Protocol Buffers decoders. Size limits before decoding. A decode error closes that connection only. Fuzz tests on client messages. | Decided. The fuzz test exists. | Proposal 0001, strategy 7.4 |
| Session token leaks through logs. | The game ticket is sent in `Hello`, not in the URL, and the client gets it in a response body. Logs never contain passwords, tokens or tickets. | Done. | Proposal 0001, decision 0004, strategy 7.8 |
| A game ticket is stolen or replayed. | Tickets are 256-bit random values, stored as SHA-256 hashes, used up by one `DELETE ... RETURNING`, and expire after 60 seconds. Logout deletes a session's unused tickets. | Done. | Decision 0004 |
| Traffic analysis of compressed frames reveals whispers. | WebSocket compression is off. | Decided. | Proposal 0001 |
| Tracking players across sessions by ID. | Entity IDs are per connection. Database IDs are never sent. | Decided. | Proposal 0001 |
| Speed hacks and teleporting. | The server checks every move against walk speed and path. | Decided. | Strategy 7.5 |
| Attacking faster than allowed. | Server-side cooldowns, cast times, and delays. Client timing is ignored. | Decided. | Strategy 7.5 |
| Seeing hidden players or monsters out of range. | The server sends only what the player can see. Other players' HP is a fraction. | Decided. | Strategy 7.5, proposal 0001 |
| Replaying a command, such as a trade confirmation. | TLS stops replay from outside the connection. Commands that move items or zeny run in database transactions and check current state, so repeating one does nothing. | Decided. | Strategy 4.1 |
| Expensive commands exhaust CPU, such as long path searches. | Path length is capped, as in rAthena's `MAX_WALKPATH`. Per-command rate limits. | Decided. | Strategy 7.4 |
| Walking into new regions forces map generation. | Generation runs in a worker pool with a queue limit. Generated maps are cached. Players cannot pick seeds. | Open. Limits need numbers. | Strategy 8.5 |

### Accounts

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| Password reuse and brute force. | Argon2id hashes (64 MiB, 3 passes, 4 threads, about 50 ms), rehashed on login when the parameters change. Common passwords are refused. Rate limits per address and per account name, with waits that double up to 15 minutes. A captcha hook after 3 failures for a name or 10 from an address. Optional TOTP codes, with replay protection and the same rate limits, and passkeys. | Done, except a real captcha provider. The limits are per process. | Strategy 7.3, decisions 0004 and 0005 |
| Learning which accounts exist. | Login gives one message for a wrong name or password, and hashes once for unknown names, so timing matches. Signup says a name is taken, which cannot be avoided, and is rate limited. | Done. | Decision 0004 |
| Locking a player out with failed logins. | The per-name wait is capped at 15 minutes and forgotten after an hour. The captcha and address limits make many failures costly. | Done. Accepted risk. | Decision 0004 |
| A client fork collects passwords. | Login happens on the account service's own page. The client receives a one-time game ticket, never the password or the login cookie. Tickets go only to trusted client origins, through CORS. Passkeys are bound to the account service's domain, the host of `--account-url`. | Done. | Strategy 7.6, decisions 0004 and 0005 |
| Stolen session tokens. | 256-bit random tokens in an `HttpOnly`, `SameSite=Strict` cookie, `Secure` with the `__Host-` prefix over https. The database stores only hashes. Sessions end after 30 days, 7 days unused, or logout. A login replaces the browser's older session. Revoking a session closes the game connections it opened. Changing login methods needs a login from the last 10 minutes. Password change revokes all sessions. | Done, except password change, which does not exist yet. A session that expires does not close its open game connection yet. | Strategy 7.3, decisions 0004 and 0005 |
| A stolen session cookie adds the thief's passkey or turns off TOTP. | Adding or removing passkeys, starting TOTP and new recovery codes need a login from the last 10 minutes, or a new proof with the password (and code) or a passkey. Turning TOTP off needs the password and a code in the same form. | Done. | Decision 0005 |
| A TOTP code is phished or seen, then used again. | A code is accepted once: its time step is stored in the same statement that checks it is newer. Codes are 6 digits, one step of skew either side, and rate limited like passwords. A pending login allows 5 codes in 5 minutes. | Done. A phished code can still be used once, within its minute, before the player uses it. Passkeys resist phishing. | Decision 0005 |
| TOTP secrets leak from a database copy or backup. | Secrets are sealed with AES-256-GCM under a key from the environment, bound to their account. Recovery codes are stored as SHA-256 of 80 random bits. Only `wathena_account` can read these tables. | Done. Someone who reads both the database and the environment gets the secrets. | Decision 0005 |
| A cloned passkey. | The signature counter must go up. A passkey whose counter does not is blocked. | Done. Synced passkeys report no counter, so this cannot detect their copies. | Decision 0005 |
| Script injection on the account pages. | Only three pages load a script: one same-origin file, with `script-src 'self'` and no inline scripts or eval. Other pages allow no script. | Done. | Decision 0005 |
| Staff account takeover. | A staff role counts only for a session that logged in with a TOTP code, a recovery code or a passkey, on an account that still has TOTP or a passkey on. The admin API checks this on every call. In the game, the game ticket records it for the session. A stolen password alone gets no role. | Done for the admin API, the Lua calls and the GM commands. A game session keeps its ticket's answer until the next login, so turning TOTP off during play applies then. | Strategy 7.3, 10.6, [decision 0028](../decisions/0028-staff-roles-and-audit.md), [decision 0030](../decisions/0030-moderation-and-gm-commands.md) |
| A stolen staff session cookie bans players. | A ban, and lifting one, needs a login from the last 10 minutes, as role changes do. A permanent ban needs `moderation.ban_permanent`, which only admins have by default. Every ban writes an audit row with its reason. | Done. Mutes, jails and kicks need no recent login: they end on their own or at once. | Decision 0030 |
| A banned player keeps playing or comes back. | A ban closes every game connection of the account at once. The gateway checks the ban at each game login, after it claims the character, so a ban during a login closes it too. The account service refuses the login once the password is right, and refuses game tickets. Only the game role writes bans. The account role reads only the columns the check needs. | Done. A ban does not end the account's web sessions: they stay, but get no game ticket. IP bans are left for later. | Decision 0030 |
| A stolen staff session cookie changes roles. | Giving and removing roles needs a login, or a proof on the reauth page, from the last 10 minutes, as login method changes do. | Done. | Decision 0028 |

### Game integrity

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| Item duplication. | Transactions for trades, storage, mail, and vending. Serial numbers on every item, with a job that flags duplicates. Database constraints on amounts. | Done for saving: every inventory row has a UUIDv7 serial as its primary key, one stack per slot, amounts of 1 to 30000, and a character and its inventory are saved in one transaction. Trades, the item log, and the duplicate job are decided. | Strategy 3.5, 7.5, decision 0003 |
| Zeny overflow. | Checked math, and database constraints that reject negative or overflowing amounts. | Database constraints done: zeny is 0 to 2147483647, exp and points are 0 or more. Checked math is decided. | Strategy 7.5, decision 0003 |
| A stale session overwrites newer progress, such as after a second login. | One session per character: a second login kicks the first and waits for its last save. A save version on each character rejects saves from an older session, and the zone then disconnects that session's player. | Done. | Decision 0003 |
| SQL injection. | Game code only runs queries that sqlc generates, which bind every parameter. A CI check rejects `fmt.Sprintf` and raw query calls in database code. | Done. | Strategy 7.2, decision 0003 |
| Predicting drops and critical hits. | Gameplay randomness uses ChaCha8 per zone, seeded from `crypto/rand`. | Decided. | Strategy 8.3 |
| Predicting secret generation, such as loot or quest answers. | The world secret never leaves the server. Child keys come from HMAC-SHA256, so public seeds reveal nothing about secret branches. Secret branches use ChaCha8. | Done for the seed tree, with test vectors. | [rng-v1](../specs/rng-v1.md) |
| Leaking the world secret through logs. | `rng.Node` prints as redacted. Keys are only readable through `Key()`. | Done. | `shared/rng` |

### Mods and the Lua runtime

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| A mod script escapes the Lua sandbox through a memory bug in C Lua or the binding. | Scripts cannot load bytecode, which is the easiest way to break C Lua. No `io`, `os`, `debug`, or `package` libraries, and no C modules. The binding stays small and is fuzzed. The Lua version is pinned, and new Lua releases are reviewed for security fixes. | Decided. | Decision 0002, strategy 6.6 |
| A mod uses all CPU or memory. | A per-call CPU budget from an instruction count hook. A memory limit from the binding's allocator. Mods that fail too often are disabled in that zone. | Done in the benchmark binding. | Decision 0002 |
| One mod reads or changes another mod's data. | Separate environments and read-only shared tables. No permission for other mods' data. | Decided. | Strategy 6.6 |
| A mod calls internal services over the network. | `net.http` is a flagged permission. Only hosts listed in the manifest are allowed, never private IP ranges. | Decided. | Strategy 7.7 |
| A malicious or changed mod package. | Signed releases and checksums, checked by `wactl`. The server owner reviews permissions. Indexes hold metadata, not code. | Decided. | Strategy 6.10 |
| A mod quietly gives items or zeny. | These actions need a permission the owner approves. Item and zeny creation is logged by serial. | Decided. | Strategy 6.6, 7.5 |

### Staff and operations

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| A staff member creates items or reads data without cause. | Role-based permissions from a fixed list, checked on every admin API call. An audit log of every action, written in the transaction of the change. A reason is required for item and zeny creation, and such rows are marked for review, by a database check. Player detail views and IP searches write audit rows. | Done for roles, the admin API, player search and details, item restores, sanctions, reports and GM commands, with their audit rows. `@item` needs `items.create` and a reason, and its row is marked for review. | Strategy 7.8, 10.6, decisions 0028, 0029 and 0030 |
| A player uses GM commands, or a staff member one their role does not allow. | Commands are parsed and checked on the server, against the role that counts for the login, with the two-factor rule. A command without the permission answers "Unknown command.", as one that does not exist, so a player cannot tell which commands exist. Command lines never go out as chat or reach the chat log. | Done. | Decision 0030 |
| A staff member abuses `@hide` to watch players unseen. | `@hide` needs `commands.hide`, and each use writes an audit row. As rAthena, other staff do not see a hidden staff member either. | Done. The row says when hiding started and ended, not what the staff member saw. | Decision 0030 |
| A staff member sanctions a player without cause, or lifts their own sanction. | Each sanction needs its kind's permission and a reason the player sees, and writes an audit row. No one sanctions their own account or lifts its sanctions. A lift needs a reason and writes a row too. Reports close with an outcome and a note, and a sanctioned report links its sanction. | Done. A friend can still lift a staff member's ban. The audit log shows who did. | Decision 0030 |
| A staff member restores items to a friend, or makes items or zeny. | `items.restore` only moves serials that exist. A copy of a gone serial and any zeny need `items.create`, which only admins have by default. Every restore needs a reason and a login from the last 10 minutes, writes an audit row marked for review in its own transaction, and logs each move with the staff account as the end of copies and zeny. Another staff member reviews the row with ok or flag. No one reviews their own row. | Done. A reviewer cannot undo a restore: a flag only marks it. Two staff members who agree can still restore and review for each other. | Decision 0029 |
| A restore duplicates a serial, such as one the holder trades or carries out at logout. | Each serial moves through the transfers, which check that its slot still holds it. An online holder's items move on its zone's goroutine, which cancels the holder's trade. An offline holder's characters are held in the store, so no login reads them until the restore's commit is queued. Storages that are open are left alone. The mail, the holders, the log lines and the audit row are one commit, so one transaction writes them. | Done. Unit tests, store tests and an end-to-end duplication test with bots that trade, use storages and carts, and lose their connections while restores run. | Decision 0029 |
| A staff member tracks players through their IP addresses. | The IP search needs `players.read_ip`, which only admins have by default. Each IP search writes an audit row with the range. Login records with IP addresses are kept 90 days, then the account service deletes them. The search results show only the logins that matched. | Done. Search results are not limited to wide or narrow ranges: an admin can search a single address. | Strategy 7.10, decision 0029 |
| The admin API's account role reads game data. | Game data comes through `admin.World` from the world service, which reads it with the game role. The account role reads only accounts, characters, login records and the audit log. | Done for the all-in-one build. | Decision 0029 |
| A staff member gives themselves or a friend more power. | Owners come only from the config. No one changes their own role or an owner's. A caller gives or takes only roles whose permissions it holds all of. Every change writes an audit row. | Done. With the default roles, an admin holds every permission, so admins can make other admins. | Decision 0028 |
| A staff member or a compromised service hides their tracks in the audit log. | `wathena_game` and `wathena_account` can only add audit rows: neither can update, delete or truncate them, by database rights. Reviews go in their own table, so a review never changes a row. Rows keep the account's name and have no foreign keys, so they outlive accounts. | Done. The table's owner, `wathena_migrator`, can still change rows. The 2-year deletion job comes with phase 8. | Decision 0028 |
| A cross-site page calls the admin API as a staff member. | `SameSite=Strict` session cookie. Every change needs the CSRF token of `/me` in the `X-CSRF-Token` header, an HMAC of the CSRF and session cookies. A `Sec-Fetch-Site` or `Origin` header, when the browser sends one, must name the service's own origin. The API sends no CORS headers, so other origins cannot read its answers. | Done. | Decision 0028 |
| Players download or probe the admin tools. | The admin screens are a separate bundle that the `/admin` page loads only after `/me` returns a role. The game page never loads it. Every call checks the permission on the server, so the bundle holds no secret. Calls are rate limited per address. | Done. | Strategy 10.6, decision 0028 |
| The world misses a role change and keeps a removed role. | The admin API tells the world through `admin.World` after the commit. In the all-in-one build that is a call in the same process. The database is the source the world loads at start. | Done for the all-in-one build. A network form for the distributed build is open. | Decision 0028 |
| A compromised service reaches all data. | One PostgreSQL role per service, with only the tables and rights it needs. Mutual TLS between services. | Done for the game and account roles: `wathena_game` has no DDL rights, cannot delete accounts or characters, and cannot read passwords or sessions. `wathena_account` alone reads passwords, and cannot change or delete characters. Migrations run as `wathena_migrator`. Other services' roles and mutual TLS are decided. | Strategy 7.8, decisions 0003 and 0004 |
| The database password leaks through process lists or logs. | `waserver` and `wactl migrate` read the database URL from an environment variable. The URL is never logged, and parse errors do not quote it. | Done. | Decision 0003 |
| Secrets committed to the repository. | Secrets come from environment variables or a secret manager. | Decided. | Strategy 7.8 |
| Lost data. | Encrypted backups and scheduled restore tests. | Decided. | Strategy 7.8 |

### Web

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| Script injection through chat, names, or NPC text. | Text is shown with `textContent`, never `innerHTML`. A strict Content Security Policy. | Decided. | Strategy 7.6 |
| Malicious guild emblem files. | Size limits, decoding, checks, and re-encoding. The original file is never served. | Decided. | Strategy 7.6 |
| Cross-site requests. | `SameSite=Strict` cookies. Every account form has a CSRF token bound to the session, and posts with a foreign `Origin` or `Sec-Fetch-Site` are refused. The ticket endpoint answers only trusted origins. The gateway checks the WebSocket `Origin` against the trusted client origins, with the scheme. | Done. | Strategy 7.4, 7.6, decision 0004 |
| Script injection on the admin screens through names, reasons or audit arguments. | The screens set text with `textContent` only. They load from the client's origin under its Content Security Policy, which allows no inline scripts. | Done. | Decision 0028 |
| Script injection or framing on the account pages. | The pages have no scripts. `Content-Security-Policy` allows no scripts, no frames, and no framing, and forms only post to the service. Templates escape every value. | Done. | Decision 0004 |
| Anyone plays any character on a development server. | `waserver --dev` refuses a listen address other than loopback. | Done. | Decision 0004 |

### Asset build and serving

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| A crafted GRF, art pack, or mod pack exploits a parser. | Length checks before every read, fuzz tests, and the build runs offline as its own command, not in the game server. The client's compiled Lua tables are never run: an evaluator reads only table-building instructions, with a step limit. | Decided. | Strategy 10.1 |
| A crafted asset name escapes the build's output folder. | Names are checked against the rules in [assets-v1](../specs/assets-v1.md) before use. Output files are named by content hash, not by asset name. | Decided. | assets-v1 |
| Path traversal or wrong content types on the asset server. | Files are served only by valid content hash, with a fixed content type per extension and `nosniff`. No directory listing. | Decided. | Strategy 10.1 |
| Hotlinking and bandwidth abuse. | Optional signed cookie from the account service. A CDN absorbs traffic. | Decided. | Strategy 10.1 |
| A stale or poisoned cache in players' browsers. | Names are content hashes, so a cached file always matches its name. The client checks the hash of each file it downloads. | Decided. | Strategy 10.1 |

### Availability

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| Connection floods. | A proxy or load balancer in front. Connection limits per IP. Internal ports are not public. | Decided. | Strategy 7.4 |
| Message floods. | Rate limits per connection and per message type. | Decided. | Strategy 7.4 |
| A crash in one connection stops the server. | Each connection goroutine recovers from panics. | Decided. | Strategy 7.4 |
| A crash in C Lua stops the zone process. | A C crash cannot be recovered in Go. The binding checks inputs, and scripts cannot reach raw memory. A supervisor restarts the process. | Open. The restart and state recovery plan is not written. | Decision 0002 |

### Supply chain

| Threat | Mitigation | Status | Reference |
|---|---|---|---|
| A compromised GitHub Action. | The repository has no workflows. | Not applicable. | [Decision 0026](../decisions/0026-local-checks.md) |
| A compromised download: Lua, zig, TinyGo, wasmtime, or wasi-sdk. | The Dockerfile and the build scripts check each download against a pinned SHA-256. Developers install the local tools themselves, and the header of `scripts/check.sh` lists the pinned versions and hashes to check them against. | Partial. | `Dockerfile`, `scripts/check.sh`, `tools/luabench/wasm/build.sh` |
| A vulnerable Go module. | `go.sum` pins modules. `govulncheck` and `gosec` run in `scripts/check.sh` before every push. | Done. | Strategy 7.9 |
| A tampered release. | Signed release binaries and a software bill of materials. | Decided. | Strategy 7.9 |

## Findings from phase 0b

These came up while writing this document.

1. **Per-mod memory limits need separate Lua states.** Strategy section 6.6 puts every mod in the same zone Lua state, with separate environment tables. The binding's memory limit applies to a whole state, so it cannot enforce the 16 MB per mod in strategy section 3.8. The fix is one Lua state per mod per zone. That fits `require`, since each mod then has its own module cache. The cost is about 1 ms and 0.2 MB per mod state (decision 0002). This belongs to the Lua API redesign.
2. **Third-party mods could run in WebAssembly.** The binding's C code also builds to WebAssembly, where a memory bug cannot reach the host. Mods the owner does not fully trust could run there, and the core mod through cgo. Decision 0002 measured WebAssembly as slower, mostly on errors and coroutine yields. This is an option to measure again, not a decision.
3. **C Lua crashes need a recovery plan.** A zone process that crashes loses unsaved state since the last save. The persistence plan saves changed data at intervals (strategy section 4.1, item 5), so the loss is bounded, but the interval and the restart behavior need numbers.

## Out of scope

- Attacks on the player's own device or browser.
- Attacks on the hosting provider.
- Whether an operator has the right to serve the game assets they supply.
