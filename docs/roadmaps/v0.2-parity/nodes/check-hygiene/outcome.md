# Outcome: check-hygiene

## Built

- `scripts/check.sh` removes its PostgreSQL container with `docker rm -f -v`, so the anonymous data volume of `postgres:18` goes with it.
- gosec runs with `go run github.com/securego/gosec/v2/cmd/gosec@$gosec_version` (`gosec_version=v2.30.0`), like buf, sqlc, golangci-lint and govulncheck. `tools/gosec` is gone: gosec v2.30.0 reads Go 1.27.2's export data.
- gosec v2.30.0 adds 6 G115 reports in tools/luabench/wasm.go, all wasm stack conversions. They carry `#nosec G115` notes in the repo's style.

## Deviations

- The security step already failed on main with the old gosec: G602 at tools/convert/npcpresent.go:1375, a false positive from commit cea8508. It is fixed on its own branch, `fix/gosec-showdigit`, so it can merge without this node.

## Stubs left

- None.

## Gotchas

- The leak was small: 11 dangling volumes of 0 to 125 MB, about 425 MB in all. It is not what fills the disk; Go caches, Docker images and the kRO asset build are larger. Existing dangling volumes are not removed; `docker volume prune` does that.
- Until `fix/gosec-showdigit` merges, the security step reports the npcpresent.go issue.

## Proposed learnings

- Tools that `go run` fetches are pinned by a version variable at the top of scripts/check.sh | one place to bump them | scripts/ | no

## Verification

- A container started and removed the old way left one more dangling volume (11 to 12); the new way left none (11 to 11).
- `bash -n scripts/check.sh` passes.
- gosec v2.30.0 over `./...`: 7 issues before the notes, of which 6 in luabench and the npcpresent one; `./tools/luabench/...` now has none. The old pinned gosec found only the npcpresent issue.
- `go vet ./tools/luabench` passes.
- Not run: the full `scripts/check.sh`, to keep the run light.

## Size

- Estimated S, actual S.
