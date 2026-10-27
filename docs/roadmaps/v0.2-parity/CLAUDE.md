# Roadmap: v0.2-parity

This folder is the project's roadmap and the only place that tracks status. `GOAL.md` has the outcome. Each node is a folder under `nodes/`, and its `status` field is the status.

Rules for agents working on a node:

- One node at a time. Never start a second worker or build while one runs: builds can use more than 20 GB of disk.
- Work only on your own node. Edit files inside your brief's owned and shared paths and your own `nodes/<id>/` folder.
- Do not edit other briefs, `status` fields, contracts or `GOAL.md`. The orchestrator owns them.
- If a contract is wrong, write `contracts/<name>.change-request.md` and adapt locally. Do not edit the contract.
- Take the next free decision, review decision, proposal and migration number when you start, and write it in your outcome.
- Regenerate generated files (`docs/generated`, `schema/gen`, `server/store/db`) in your branch after rebasing on main.
- Record what you built in a decision record, as the repo always has. Do not add progress paragraphs to docs/strategy.md.
- Keep notes in your `log.md`. Finish by writing `outcome.md` from the template.
- Read the outcomes of your direct dependencies. Do not read other nodes' logs.

Script: `python3 <skill>/scripts/plan.py docs/roadmaps/v0.2-parity --max-parallel 1 <command>`.
