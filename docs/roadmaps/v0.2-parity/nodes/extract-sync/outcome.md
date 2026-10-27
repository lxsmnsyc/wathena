# Outcome: extract-sync

## Built

- Every file under reference/rathena/extracts reproduces from the rAthena fork's `wathena-reference` branch at 39de3053d: `python3 tools/wathena/extract.py` writes 34 renewal and 30 pre-renewal files (part files of `npcs.json` included). All 64 are byte identical to this repository's files except one line, `rathena_git_hash`, which holds the fork commit that wrote the file. The data was not copied again.
- reference/rathena/README.md: Source notes name 39de3053d and e98500617 (the rAthena data commit the branch is based on), list the fork commits that added the recorder and each extract function, and drop every "does not have yet", "not in the fork yet" and "kept outside this repository" note. Also corrected `refine_db.json` (has Normal costs) and listed `quests.json`, `item_trade.json` and `shops.json`.

## Deviations

- None. No extract file was changed (the only difference is the hash field).

## Stubs left

- None.

## Gotchas

- The `wathena-reference` branch exists only in the local checkout ~/Documents/GitHub/rathena. It has no upstream and is not on github.com/lxsmnsyc/rathena. Maintainer decision: push it (or keep a bundle), or the recorder and extractor have a single copy.
- Not checked, needs a C++ build: the recordings in reference/rathena/renewal and pre-renewal (`record.sh`, built map server). `inventory/` (`inventory.py`) was not run either. These carry hash e98500617 or c1dfe21b8.
- `rathena_git_hash` in an extract changes with every fork commit, so a byte comparison needs it masked.

## Proposed learnings

- Compare extracts with `rathena_git_hash` masked | the field is the fork HEAD at run time | reference/rathena | no

## Verification

- `python3 tools/wathena/extract.py` in the fork at 39de3053d: ran, no errors. Fork `generated/wathena` restored; fork status clean.
- 64 of 64 files identical with the hash field masked.
- `go test ./reference/... ./tools/convert`: ok.

## Size

- Estimated S, actual S.
