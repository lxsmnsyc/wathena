# 0026: Local Checks Without CI

- Status: proposed.
- Date: 2026-10-05.
- Code: [scripts/check.sh](../../scripts/check.sh), [scripts/fuzz.sh](../../scripts/fuzz.sh), [scripts/sqlcheck.sh](../../scripts/sqlcheck.sh), [scripts/compose-smoke.sh](../../scripts/compose-smoke.sh).
- Builds on decisions [0017](0017-e2e-and-transfers.md), "Local checks", and [0022](0022-test-speed.md).

## Decision

### No workflows

- `.github/workflows/ci.yml` and `fuzz-nightly.yml` are removed. GitHub Actions did not run for this repository, and `scripts/check.sh` already ran every job locally.
- The scripts in `.github/scripts` move to `scripts/`.
- The tool versions and SHA-256 hashes that `ci.yml` pinned for its downloads are listed in the header of `scripts/check.sh`. The script does not install tools, so the hashes are for checking a local install by hand.
- The nightly fuzz run of 5 minutes per test is gone. `scripts/fuzz.sh 5m` runs it by hand.

### Two levels of checks

- `scripts/check.sh quick` runs `vet`, `unit` and `lint`, before each commit.
- `scripts/check.sh` runs the default steps, before a push or a review.
- `unit` is a new step: the first pass of `test`, `go test` without `-race` on every package but `e2e`. `test` runs it, then the `-race` pass.
- `unit` does not start PostgreSQL. A container's port changes each run, which changes `WATHENA_TEST_DATABASE_URL` and makes Go's test cache miss for every package that reads it. The database tests skip unless the variable is set.
- Other steps can follow `quick`, such as `scripts/check.sh quick client`.

## Results

Measured on 2026-10-05 on an Apple M2 with 8 GB of memory and 8 cores.

| Run | Time |
|---|---|
| `quick`, nothing changed since the last run | 5 s |
| `quick`, after a change in `server/zone` | 80 s |
| `go test -count=1` on `server/zone` and the 10 packages that import it | 36 to 51 s |
| `unit` with no cached results | 68 s at best, 372 s and 1,194 s in two other runs |

- The time of an uncached run varies a lot on this machine. `tools/modkit` alone took 32 s in one run and 957 s in another. The slow runs happened while swap use was over 7 GB of 9 GB, and their CPU use was near one core, so they waited and did not compute. The cause is not found.
- Limiting `go test -p` did not help: the full uncached pass took 68 s with 8 packages at once and 1,194 s with 3.
- `server/zone` and `tools/modkit` each use up to about 2 GB of memory in their tests.

## Other choices

- Running the `-race` pass only on the packages that start goroutines. The `-race` pass takes about 375 seconds, and `e2e` alone bounds it at 374, so a shorter list saves little. A list kept by hand could also miss a new package that starts goroutines.
- Limiting how many packages `go test` runs at once by the machine's memory. See "Results".
- Testing only the packages changed since the last commit. Go's test cache already reruns only the packages whose inputs changed, and the packages that import them, and it never misses a dependent.
