# Setting up a wAthena server

This guide takes you from an empty Linux machine to a running world. It uses the Docker Compose stack in `deploy/compose`: `waserver`, PostgreSQL 18 and Caddy for TLS.

- Time: about 15 minutes of your own work for a procedural world, after Docker is installed. The commands of steps 3 to 10 ran in under 2 minutes on the test machine (2026-10-20). Building the images took 7 minutes more, and importing a full game client takes about an hour.
- You need to know how to use a shell and edit a text file. You do not need Go or Node.
- [deploy/README.md](../../deploy/README.md) is the reference for the stack, and [server/README.md](../../server/README.md) for every setting. This guide links to them instead of repeating them.
- Players read [players.md](players.md). Link it from your site, or copy it and fill in your server's name.

## What you need

The machine:

- Linux on amd64 or arm64, with Docker Engine and Docker Compose 2.20 or later.
- For 250 players online: 4 CPU cores, 8 GB of memory and 50 GB of SSD. This is the number the project measured, on an Apple M2 with 8 GB that also ran desktop apps and Docker (strategy section 3.9). The server used under half a core. Memory set the limit. For more players, measure your own machine (see "Measure your own hardware").
- A host name, such as `play.example.com`, whose DNS record points at the machine.
- Ports 80 and 443 open to the internet, TCP and UDP for 443. Caddy needs port 80 to get a certificate from Let's Encrypt.

Your game files:

- wAthena ships no game art. It ships a small placeholder pack of its own, which is enough to check that the server runs.
- For real art, and for a classic world, you need your own Ragnarok Online client: a folder with `DATA.INI` and the GRF files it names.
- The import needs 4 GB of memory and about 3 GB of disk for the output, on top of the client itself.

Players need a Chromium-based browser, such as Chrome, Edge or Brave. Firefox and Safari are not supported.

macOS works for a test, with Docker Desktop or Colima. Clone into a folder under your home folder, such as `~/wathena`. Docker on macOS shares only your home folder with its virtual machine by default, and the stack mounts files from its folder.

## 1. Install Docker

On Debian or Ubuntu, Docker's install script installs Docker Engine and Compose:

```sh
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"   # then log out and in again
docker compose version
```

Expected output, or a newer version:

```text
Docker Compose version v2.20.0
```

## 2. Get wAthena

Releases are on the [GitHub releases page](https://github.com/lxsmnsyc/wathena/releases). There is no tagged release yet. Until there is one, use the `main` branch and build the images on the server:

```sh
git clone https://github.com/lxsmnsyc/wathena.git
cd wathena/deploy/compose
```

Once releases exist:

- Clone the release's tag instead: `git clone --branch v0.1.0 https://github.com/lxsmnsyc/wathena.git`.
- Each release has the binaries `waserver`, `wactl` and `wabots` for linux/amd64, linux/arm64, darwin/arm64 and windows/amd64, and a `SHA256SUMS` file. You do not need them for the Compose stack. `wactl` and `wabots` are useful on your own machine, for backups and load tests.
- `scripts/release.sh` builds the same binaries from source. It needs Go, zig 0.16.0, Node and pnpm.

Run every command from here on in `wathena/deploy/compose`, unless a step says otherwise.

## 3. Settings

Copy the example settings and set your host name:

```sh
cp .env.example .env
sed -i.bak 's/^WATHENA_DOMAIN=.*/WATHENA_DOMAIN=play.example.com/' .env && rm .env.bak
grep ^WATHENA_DOMAIN .env
```

- The last command prints `WATHENA_DOMAIN=play.example.com`. You can also edit `.env` in an editor.
- `.env.example` explains every setting. The defaults fit a public server.
- For a test on your own machine, see "Test on your own machine" below.

Then open `waserver.yml`. Its last lines are a privacy block, commented out. Replace them with your own. Players see it on `/account/privacy`:

```yaml
privacy:
  contact: privacy@example.com
  text: |
    This server is run by Example Team. We keep your account name,
    the IP addresses of your logins, your chat and your game data
    to run the game.
```

You are responsible for following the laws that apply to you, such as the GDPR.

## 4. Secrets

```sh
./secrets.sh
```

Expected output:

```text
made secrets/postgres_password
...
made secrets/account_secret_key
made secrets/backup_recipients
note: secrets/backup_recipients has no recipient yet, so the backup service makes no backups until you add one and run: docker compose up -d --force-recreate backup
secrets are in /home/you/wathena/deploy/compose/secrets
```

- It writes random database passwords and the key that encrypts two-factor secrets into `secrets/`. Git ignores the folder.
- Running it again keeps the files that exist.
- Back up `secrets/`, and above all `secrets/account_secret_key`, somewhere other than the server. Without that key, players with two-factor login must use a recovery code.

## 5. Build the images

```sh
docker compose --profile tools build
```

- This builds `wathena/waserver:local` and `wathena/wactl:local` from the repository. It downloads the base images and the Go and Node packages first.
- It took 7 minutes on the test machine, an Apple M2 with 8 GB, with some base images already downloaded. Expect 5 to 15 minutes, and about 3 GB of disk for the images and the build cache.
- It ends with lines such as `Image wathena/waserver:local Built`.
- A second build reuses the cache. With no change, it takes under a minute.

## 6. Choose a world mode

A world's mode is fixed when you make it. Choose one now.

| Mode | What it is | What you need |
|---|---|---|
| Procedural, the default | Generated fields, towns and dungeons from a secret seed. New content, no fixed maps. | Nothing more. It runs with the placeholder pack, or with your client's art. |
| Classic | rAthena's maps, NPCs and monster spawns, run from your client's GRF files. | Your game client. |
| Pre-renewal classic | Classic with rAthena's pre-renewal rules. | Your game client. It is experimental, and `waserver` warns at start. |

Classic mode does not run every rAthena NPC yet. [docs/generated/classic-coverage.md](../generated/classic-coverage.md) lists what does not convert. In short:

- 4,982 of 15,337 NPC lines are placed, with every Kafra and 264 job change NPCs.
- Instances, guild sieges, battlegrounds and the MVP tomb are not built.

## 7. Assets

The server streams the game's art to players' browsers from an asset build. Build it into the stack's `assets` volume with the `offline` service, which has no network and no secrets.

With the placeholder pack, for a first test of a procedural world:

```sh
docker compose run --rm offline assets build --source placeholder --out /srv/assets
```

With your own game client:

```sh
docker compose run --rm -v /path/to/client:/client:ro \
  offline assets build --source grf:/client --out /srv/assets
```

- `/path/to/client` is the folder with `DATA.INI`.
- By default the build compresses with Brotli's highest level, so players download the smallest files. It is slow.
- `--fast` uses a lower level: a quicker build and larger downloads. A full kRO client took 57 minutes with `--fast` on an Apple M2 with 8 GB: 112,792 assets and 988 maps, 2.8 GB on disk. Use it for a first test, and build again without it before you open the server.
- Run the same command again after a client update. Unchanged assets come from the build's cache.
- A placeholder build for a classic world needs `start_map: placeholder/map` in the `classic` block and `mode: none` in `waserver.yml`. Use your client instead for a real classic world.

## 8. Make the world and your account

The account you make here is the world's owner. Pick a name of 4 to 24 letters, digits and underscores, starting with a letter. The password needs at least 8 characters and must not be a common one.

```sh
read -rs PASSWORD   # type the password, then Enter
printf '%s\n' "$PASSWORD" | docker compose run --rm -T wactl world new \
  --name "My World" --admin Admin --admin-password-file -
unset PASSWORD
```

- The password goes through standard input, so it stays out of the shell history and the process list.
- For a classic world, add `--mode classic`.
- For pre-renewal classic, add `--mode classic --ruleset pre-renewal`, and set `mode: pre-renewal` in `waserver.yml`. `--ruleset` must always match `mode` in `waserver.yml`.

It starts PostgreSQL, applies the database migrations, and makes the world. This takes about 30 seconds. Expected output for a procedural world, shortened:

```text
applied 00001
...
applied 00041
made the procedural (generator 3, rng-v1) world "My World" (renewal) and the account Admin (id 1)
the layout: fields 30, towns 5, dungeons 6 with 20 floors, links 66
towns: Svalvik (t1), Hjalhold (t2), Grimunby (t3), Lordor (t4), Ysarfjord (t5)
...
The account Admin (id 1) is the world's owner. It needs no staff.owners line in waserver.yml.
```

Each world gets its own seed, so your names and numbers differ.

## 9. The backup key

Backups are encrypted to a key that only you hold. Make it on your own computer, not on the server, with [age](https://age-encryption.org):

```sh
age-keygen -o backup-identity.txt
```

- It prints `Public key: age1...`. That public key is the recipient.
- Keep `backup-identity.txt` off the server, such as in a password manager. Without it, no backup can be restored.
- Install age with `sudo apt install age` or `brew install age`.
- A release's `wactl world backup-key new --out backup-identity.txt` does the same. It prints the recipient on its first line.
- Without age or `wactl`, a computer with Docker and the `wathena/wactl` image can make the key. Docker cannot write the file as your user, so the image prints it:

  ```sh
  (umask 077; docker run --rm --network none --entrypoint sh wathena/wactl:local \
    -c 'wactl world backup-key new --out /tmp/key.txt >&2 && cat /tmp/key.txt' > backup-identity.txt)
  ```

  The recipient is the `# public key:` line of the file. If you run this on the server, move the file to your own computer at once and delete it on the server.

On the server, add the recipient on a line of its own:

```sh
echo age1yourpublickey >> secrets/backup_recipients
```

## 10. Start

```sh
docker compose up -d --wait
```

It waits until every service is healthy. The first start took 30 seconds on the test machine.

Check it:

```sh
docker compose ps
docker compose exec waserver waserver health && echo ready
```

- `docker compose ps` lists four services. `backup` has no health check:

  ```text
  SERVICE    STATUS
  backup     Up 26 seconds
  caddy      Up 10 seconds (healthy)
  postgres   Up About a minute (healthy)
  waserver   Up 26 seconds (healthy)
  ```

- `waserver health` checks `/readyz`: the world runs and both databases answer. It prints `ready`.
- Caddy answers `/readyz` and `/healthz` with 404 from the internet, on purpose.

Then open `https://play.example.com/` in Chrome or Edge:

1. Sign up, or log in as the owner account.
2. Make a character and press Play.
3. The game loads and your character stands in the first town.

If a step fails, see "Troubleshooting".

## Test on your own machine

To try the stack on a laptop before you rent a server:

```sh
cp .env.example .env
sed -i.bak -e 's/^WATHENA_DOMAIN=.*/WATHENA_DOMAIN=localhost/' \
  -e 's/^WATHENA_HTTP_PORT=.*/WATHENA_HTTP_PORT=8080/' \
  -e 's/^WATHENA_HTTPS_PORT=.*/WATHENA_HTTPS_PORT=8443/' .env && rm .env.bak
```

- Caddy then signs its own certificate for `localhost`. Open `https://localhost:8443/`.
- The browser warns about the certificate until you trust Caddy's local authority. Without that trust, the client's Service Worker does not register. After step 10, copy the authority out and add it to your system's or browser's trust store:

  ```sh
  docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt caddy-root.crt
  ```

- Follow the other steps as written.

## After the first start

### Secure the owner account

The owner role, and any staff role, only counts after a login with a second factor.

1. Open `https://play.example.com/account/settings`.
2. Turn on two-factor login with an authenticator app, and save the recovery codes. Or add a passkey that asks for a PIN or a fingerprint.
3. Log out, and log in again with the code or the passkey.
4. Open `https://play.example.com/admin`. The admin screens show the tabs of your role.

A passkey that does not check a PIN or fingerprint logs you in, but without the staff role.

### Give staff roles

On the admin screens, the Staff tab gives and removes roles. The default roles are `admin`, `game_master` and `moderator`.

- Each staff member also needs two-factor login or a passkey, as above.
- `staff.roles` in `waserver.yml` changes what each role may do. server/README.md, "Staff and admin tools", lists the permissions.

### Backups

The `backup` service makes an encrypted backup once a day into the `backups` volume. It keeps one for each of the last 7 days and 4 weeks.

With the recipient of step 9 in place, the service makes its first backup when the stack starts. Check it:

```sh
docker compose exec backup ls -l /backups
```

It lists a file such as `world-20261020T030542Z.dump.age` and its manifest, `world-20261020T030542Z.json`.

- If you add or change the recipient later, the service reads it only when it starts. Run `docker compose up -d --wait --force-recreate backup`.
- `docker compose logs backup` shows each backup, and a warning every hour while there is no recipient.

To copy the backups off the server:

```sh
docker compose cp backup:/backups ./backups
```

Copy them to another machine on a schedule, such as with `rsync` or `rclone`. A backup kept only on the server is lost with the server.

Check that a backup restores. There are two ways:

- On your own machine, with a release's `wactl` and PostgreSQL 18. The key never goes to the server. deploy/README.md, "Checking backups on your own machine", has the command.
- On the server, with the `verify` profile. It restores the newest backup into a temporary database once a day. The server then holds the key, so anyone who takes the server can read the backups. deploy/README.md, "Checking backups on the server", has the steps and the warning.

To restore, see deploy/README.md, "Restoring".

### Retention

The server deletes old personal data once a day. The defaults, in days:

```yaml
retention:
  login_ip_days: 90    # login records with IP addresses
  chat_days: 30        # chat lines, except those of open reports
  item_log_days: 180   # item and zeny logs
  audit_days: 730      # the staff audit log
```

- Add the block to `waserver.yml` only to change a value. Shorter periods need no notice.
- `login_ip_days` and `chat_days` are 1 to 3650. `item_log_days` and `audit_days` are 30 to 3650.
- A longer period logs a warning and shows one on the admin World screen.
- Backups keep deleted data for up to 4 weeks. The privacy page says so.
- After any change to `waserver.yml` or `.env`, run `docker compose up -d --wait`.

server/README.md, "Retention", has the limits and the details.

### Measure your own hardware

`wabots` plays the project's load tests against a server and checks the results against the performance budgets. Run it before you open a world, to learn how many players your machine holds.

- Use a test world in a database of its own, never your live world. The bots make characters and change their levels, items and places.
- Stop the live stack while it runs, so the test measures the machine alone.
- Start with 250 bots, and raise the count by 250 while the report says `all budgets met`.

```sh
wabots --scenario fields --bots 250 --metrics http://127.0.0.1:8301 --check
```

deploy/README.md, "Measure your own hardware", has the full steps: the test database, the test `waserver` and each scenario.

The test runs the `waserver`, `wactl` and `wabots` binaries of a release on the host, not in Docker. Until there is a release, build them from the repository with Go and a C compiler:

```sh
go build -tags procedural -o waserver ./server/cmd/waserver
CGO_ENABLED=0 go build -tags procedural -o wactl ./tools/wactl
CGO_ENABLED=0 go build -tags procedural -o wabots ./tools/wabots
```

### Update to a new release

1. Read the release notes.
2. Make a backup and copy it off the server:

   ```sh
   docker compose run --rm backup world backup --dir=/backups --recipients-file=/run/secrets/backup_recipients
   docker compose cp backup:/backups ./backups
   ```

3. Get the new code, and any new secrets:

   ```sh
   git fetch --tags
   git checkout v0.2.0   # or: git pull, on main
   ./secrets.sh
   ```

4. Build the images and start again:

   ```sh
   docker compose --profile tools build
   docker compose up -d --wait
   ```

- The `migrate` service applies the new database migrations before `waserver` starts.
- `waserver` saves every character before it stops. Players see a 10 second countdown.
- To warn players earlier, plan the restart on the admin World screen first.

## Troubleshooting

Start with the logs of the service that failed:

```sh
docker compose ps -a
docker compose logs waserver
```

| What you see | Cause and fix |
|---|---|
| `the database has no world; run wactl world new` | Step 8 did not run, or failed. Run it. |
| `the database has pending migrations` | `migrate` did not finish. Read `docker compose logs migrate`, then run `docker compose up -d --wait` again. |
| `waserver` exits because `mode` is not the world's ruleset | `mode` in `waserver.yml` must match `--ruleset` of step 8: `renewal` or `pre-renewal`. |
| `waserver` exits about the assets or `manifest.json` | Step 7 did not run, or wrote somewhere else. Run it again with `--out /srv/assets`. |
| A classic world exits because the start map is missing | The asset build has no map of that name. Build from your client, or set `classic.start_map`. |
| `secrets.sh: .../secrets must be a folder of this user with mode 0700` | The folder lost its mode in a copy. Run `chmod 700 secrets`, and think about changing the passwords. |
| `bind: address already in use` for Caddy | Another program uses port 80 or 443. Stop it, or set other ports in `.env` for a test. |
| `Pool overlaps with other one on this address space` | The edge network's subnet is in use. Set `WATHENA_EDGE_SUBNET` and `WATHENA_PROXY_IP` in `.env` to a free subnet. |
| The browser cannot connect, or shows a certificate error | Read `docker compose logs caddy`. The DNS record must point at this machine, and ports 80 and 443 must be open. Let's Encrypt limits retries, so fix the cause before you try many times. |
| `bind source path does not exist: .../secrets/postgres_password` on macOS | Docker's virtual machine cannot see the folder. Move the `wathena` folder under your home folder, such as with `mv`, which keeps the mode of `secrets/`. |
| `backup` logs `backups are off` every hour | `secrets/backup_recipients` has no `age1...` line. Add it, then `docker compose up -d --wait --force-recreate backup`. |
| A staff member does not see the admin screens | The role counts only after a login with two-factor or a passkey with a PIN. Log out and in again. |
| The game page stays blank | Use a Chromium-based browser. Firefox and Safari are not supported. |

To start over on a test machine, `docker compose down -v` deletes the stack and every volume: the database, the backups, the assets and the certificates. Never run it on a live server.
