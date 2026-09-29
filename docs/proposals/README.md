# Design Proposals

A design proposal is needed before changing any of these:

- The client and server protocol.
- The public Lua API.
- The map format.
- The seed tree specification.
- The database schema.

Small changes do not need a proposal.

## Format

Create `NNNN-short-title.md`, where `NNNN` is the next free number. Include:

1. **Problem.** What is wrong or missing.
2. **Change.** What will be built or changed.
3. **Breaks.** What stops working, and how existing worlds, mods, or clients are migrated.
4. **Alternatives.** Other options and why they were not chosen.

A proposal is accepted when it is merged.
