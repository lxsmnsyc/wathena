# Premortem (2026-10-26)

It is months from now and v0.2.0 has not shipped. Why?

1. **The rAthena fork's extractor could not reproduce the extracts.** Several extracts came from functions and patches kept outside the fork. When extract-folders and the new goldens needed regenerated data, nothing matched, and every dependent node waited.
   Change: extract-sync checks byte-for-byte reproduction early. The fork (~/Documents/GitHub/rathena) already holds every function; its branch is only local, which is the remaining risk.

2. **Instances turned out larger than one L node.** 2,860 renewal scripts touch many memorial dungeons with their own mechanics; the content node kept growing.
   Change: instances-content is an outline. The curator splits it by dungeon group after instances-core lands, using the report's per-file numbers.

3. **Disk filled during a build and corrupted a run.** Builds use more than 20 GB; leaked Docker volumes add up.
   Change: strictly sequential execution (GOAL.md constraint), check-hygiene (must, first) fixes the volume leak, and workers run `docker volume prune` after checks.

4. **"Converted" was not "runs as in rAthena".** The reports reached their numbers, but players found scripts that convert and misbehave, because nothing measures behavior.
   Change: the maintainer made behavior parity a v0.2.0 requirement. npc-behavior-spike (gated) chooses the method, npc-behavior-harness builds it, and release-v0.2.0 waits for its report.

5. **Proposals sat at their gates.** Four design proposals and two must reviews need the maintainer; with one worker, every gate stops all progress.
   Change: proposals and reviews are scheduled early, and while a gate waits, the orchestrator runs the next ready non-gated node (converter, item and monster nodes need no proposal).
