# 0047: Guild Towns and the Release Build

- Status: proposed.
- Date: 2026-10-18.
- Code: [server/content/quests.go](../../server/content/quests.go), [internal/buildinfo](../../internal/buildinfo), [server/cmd/waserver/main.go](../../server/cmd/waserver/main.go), [tools/wactl](../../tools/wactl) (`main.go`, `levelpath_test.go`, `testdata/golden-content.json`, `testdata/golden-content-economy.json`), [scripts/release.sh](../../scripts/release.sh), [Dockerfile](../../Dockerfile), [deploy/compose](../../deploy/compose) (`docker-compose.yml`, `.env.example`), [e2e](../../e2e) (`classic_test.go`, `server.go`).
- Phase 8 step 1 (strategy section 15, "Phase 8"), strategy sections 11.4 and 14.2, and the first review item of decision 0046.
- Lua API: none. Protocol: none. Settings: none.

## Context

Phase 8 is the first release, built with the `procedural` tag, with both world modes (section 14.2). Its first step had two parts:

- Decision 0046 left a review item: only the Swordman guild was sure to stand in a town of a band from level 20 or lower. The other families' guilds could stand in a town of levels 41 to 50 or higher, so a Novice who wanted to be an Archer had to cross harder fields first.
- There was no release build. The Dockerfile built `waserver` and `wactl` without the `procedural` tag, and `wactl` had no version command.

## Decision

### Guild towns

`content.GuildTowns` places each class family's guild:

- The hosts are the towns of a band that starts at `content.GuildLevel` (20) or lower, by their band's lowest level, then number, the first three at most.
- A world with no such town has the lowest town as its one host.
- The hosts take the families in turn, as before: Swordman, Mage, Archer, Acolyte, Merchant and Thief.

The rule before took the three lowest towns whatever their bands. In the three worlds of `TestLevelPath`, the Archer and Thief guilds stood in a town of levels 41 to 50 or 65 to 74. Now every guild stands in the home area's town or the next low town.

The second job tests stay with their guild. A guild is one place with both stages' tests, and the guild master gives both. Moving the second job tests to a higher town would need a second place and a second NPC per family. A first job player of job level 40 is at about base level 45, and walks back to a low town without danger. The second stage's hunts still pick monsters near level 45 from the fields nearest the guild's town, further when none fit, as decision 0044 set.

### No generator version 4

The change does not touch `shared/gen`, so there is no generator version 4:

- `content` runs once, in `wactl world new`. The database keeps the result (decision 0044), and `waserver` reads the stored places and quests. A made world keeps its guilds where they are.
- The client never makes content. Map cells, WMAP files and places of generated maps, which `gen.wasm` makes, do not change, and neither do the golden files of `shared/gen`, `server/layout`, `server/spawns` and `server/rotation`.
- The content golden files of `tools/wactl` change. They fix what a new world of today's code gets, and they hold the guild towns. In both files, the Archer, Acolyte, Merchant and Thief guilds moved, and the quest checksums changed with them. Board quests, the rebirth town and the place counts are the same.

### Release build

`scripts/release.sh` builds the release binaries:

- Targets: `linux/amd64`, `linux/arm64`, `darwin/arm64` and `windows/amd64`. A list of targets on the command line builds only those.
- `waserver` with the tags `procedural`, `webclient`, `netgo` and `osusergo`, with cgo through `zig cc` (decision 0002): static musl binaries for Linux, MinGW for Windows, and the macOS SDK from `xcrun` for darwin. `wactl` with the tag `procedural` and without cgo.
- The version is `git describe --tags --always --dirty`, stamped into `main.buildVersion` with `-ldflags -X`.
- Output: `dist/<version>/`, with `waserver-<os>-<arch>`, `wactl-<os>-<arch>`, `.exe` for Windows, and `SHA256SUMS`. `dist/` is in `.gitignore`, and so is `/waserver`.
- The build runs the binaries of the host and prints their version line, as a check of the stamp and the tag.

Flags for builds that repeat byte for byte:

- `-trimpath`, `-buildvcs=false`, and `-ldflags "-s -w -buildid="`.
- `CGO_CFLAGS=-O2` and `CGO_LDFLAGS=-O2`, without Go's default `-g`. With `-g`, zig's linker writes a PDB for Windows from a temporary folder, and its path changes the PDB GUID and the time stamps in the PE headers on each build.
- `GOAMD64=v1`, `GOARM64=v8.0` and `GOFLAGS=-mod=readonly`, so the caller's environment does not change the output.
- `SOURCE_DATE_EPOCH` is the commit time, for tools that read it.

Two runs of the script on this machine gave the same `SHA256SUMS` for all 8 binaries. Builds on other machines are not compared yet.

### Version commands

- `waserver version` and `waserver --version`, and the new `wactl version` and `wactl --version`, print the version and the build's world modes, such as `v0.1.0 (modes: classic, procedural)`. The modes show whether a binary has the `procedural` tag.
- `internal/buildinfo` holds the shared code: the stamp, or the module version and VCS revision from the build information.
- waserver's log and admin tools show the version without the modes, as before.

### Docker and Compose

- The Dockerfile builds both binaries with the `procedural` tag, stamps both with `VERSION`, and runs `waserver version` and `wactl version` at the end of the build.
- Compose passes `WATHENA_VERSION` as `VERSION` to the tools image too, not only to the server image. `.git` is not in the build context, so the operator sets `WATHENA_VERSION` to the output of `git describe` for a release.

### Classic worlds in the release build

A binary with the `procedural` tag runs classic worlds. `wactl world new --mode classic` makes the world row with no generator, and `waserver` runs the maps of `--maps` with no generated maps. The e2e test `TestClassicWorldProceduralBuild` proves it with PostgreSQL.

What classic mode lacks for the release is the item of phase 8 that this step does not build: maps from the owner's GRF files, the converted NPCs, and the fixed spawns from rAthena's data.

## Tests and checks

- `server/content`: `TestGuildTowns` (new): two low towns of four, three low towns of four given out of order, one low town, and no low town. `TestGenerate` now wants every guild in the low town of its small world, whose other town is of levels 41 to 50. `TestRebirthTown` no longer checks guild towns.
- `tools/wactl`: `TestLevelPath` checks that every family's guild stands in a town of a band that starts at `content.GuildLevel` or lower, not only the Swordman guild. With the rule before, it fails on all three of its worlds. `TestGoldenContent` and `TestGoldenEconomyContent` hold the new guild towns.
- `internal/buildinfo`: `TestVersion`.
- e2e, with PostgreSQL: `TestClassicWorldProceduralBuild` (new). `Options.Classic` builds the binaries with the `procedural` tag and makes a classic world with `wactl world new --mode classic`. A new character joins on the start map, walks, and the fixture's Guide sends it to the field map. `TestProceduralContent` passes with the new guild towns.
- `scripts/release.sh`: all 4 targets build, the host binaries print their version, and a second run gives the same checksums. The Linux arm64 binaries ran in a container.

## Lua API

None.

## Left

- Signing and the software bill of materials (phase 8, a later step).
- Archives of the binaries with the license, the README and a sample config.
- A comparison of release builds on two machines.
- A Docker image build with the new flags. The disk of this machine was too full to build it in this step.

On 2026-10-20 the project chose to publish no binaries or images (strategy section 2.2). Releases are git tags, and `scripts/release.sh` is a build helper for owners. The first two items above are dropped.

## Later changes

- A release `wactl world new`, outside the repository, failed for a procedural world with "reference: go.mod not found above the working directory". Only the mod commands read the reference data built into wactl. Now every command does, and the binary did not grow (decision 0048, "Reference data in wactl"). `waserver` read the built-in data already.

## Review items

- Whether `content.GuildLevel` should be a setting of `procedural.layout`, for worlds whose bands start higher.
- Whether the release should be a set of archives per target, rather than plain binaries.
