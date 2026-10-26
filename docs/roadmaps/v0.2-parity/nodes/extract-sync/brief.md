---
id: extract-sync
title: Check the extracts reproduce from the rAthena fork and fix the README
status: active
priority: must
type: chore
outline: false
milestone: v0.2.0
size: S
spike: false
verify: go test ./reference/... ./tools/convert
gate: false
owns:
  - reference/rathena/README.md
  - reference/rathena/extracts/**
provides: []
depends_on: []
---

# Check the extracts reproduce from the rAthena fork and fix the README

## Goal

Prove that every extract in reference/rathena regenerates from the rAthena fork's `wathena-reference` branch, and bring reference/rathena/README.md up to date. The README still says several `extract.py` functions and two recorder patches are missing from the fork; on 2026-10-26 they are all committed there (`build_equipment` to `build_quests`, `PART_FOLDERS` at 39de3053d, the patches as 784fce3ec).

## Context

The fork is a local checkout at ~/Documents/GitHub/rathena, branch `wathena-reference`, HEAD 39de3053d. Its remote is github.com/lxsmnsyc/rathena, but the branch has no upstream and is not on the remote: the local checkout is the only copy. `extract.py` is Python and needs no C++ build. The recordings under renewal/ and pre-renewal/ need `record.sh` and a built map server; do not re-record in this node.

- Source: reference/rathena/README.md; decision 0044 'Left'; fork checked 2026-10-26

## Steps

- [ ] Run `python3 tools/wathena/extract.py` in the fork at HEAD and copy `generated/wathena/*` to a scratch folder, not into this repository.
- [ ] Compare each extract with reference/rathena/extracts; list differences and their cause.
- [ ] Rewrite the README's 'Source' notes to name one fork commit and drop the 'does not have yet' notes.
- [ ] Tell the maintainer the branch exists only locally, so they can decide whether to push it.

## Acceptance

- [ ] Every file under reference/rathena/extracts matches the fork's output at the named commit, or the README names each difference and why.
- [ ] `go test ./reference/... ./tools/convert` passes.

## Sizing

Python run and a README rewrite; no C++ build.

## Notes for the worker

- Do not push the fork's branch; pushing is the maintainer's call.
- Do not change converter code here.
