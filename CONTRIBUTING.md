# Contributing to wAthena

## Before you start

- Read [docs/strategy.md](docs/strategy.md) for the plan and the decisions already made.
- Changes to the protocol, the public Lua API, the map format, the seed specification, or the database schema need a design proposal first. See [docs/proposals](docs/proposals/README.md).

## Commits

- Every commit has a `Signed-off-by` line under the [Developer Certificate of Origin](https://developercertificate.org/). Use `git commit -s`.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), for example `feat(server): add zone tick scheduler`.
- Keep the summary under about 72 characters. Put details in the body as a list of sentences.

## Pull requests

- Start with a one or two sentence summary.
- List the changes as sentences.
- List how you tested the change.
- The repository has no CI. Run `scripts/check.sh quick` before each commit, and `scripts/check.sh` before you push or ask for review. The full checks must pass before review ([decision 0026](docs/decisions/0026-local-checks.md), server/README.md, "Local checks").

## Tests

- Game formulas must match the reference data in `reference/rathena` for the ruleset they implement.
- New systems need tests before they merge.
- A heavy test that runs on one goroutine, such as a parity or golden test, calls `testrace.SkipUnderRace` from `internal/testrace`. It then runs only in the pass without `-race`. A test that starts goroutines must not call it ([decision 0022](docs/decisions/0022-test-speed.md)).

## License

By contributing, you agree that your contribution is licensed under the GNU General Public License, version 3 or later.
