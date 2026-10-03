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
- The local checks must pass before review: run `scripts/check.sh`. GitHub Actions does not run for this repository, so the script runs the jobs of `.github/workflows/ci.yml` instead (server/README.md, "Local checks").

## Tests

- Game formulas must match the reference data in `reference/rathena` for the ruleset they implement.
- New systems need tests before they merge.

## License

By contributing, you agree that your contribution is licensed under the GNU General Public License, version 3 or later.
