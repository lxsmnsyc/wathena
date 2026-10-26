---
id: instances-ui
title: Instances: client window, admin screen and staff endpoint
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
added: 2026-10-27
owns:
  - client/src/world/instance*
  - client/src/admin/**
shared:
  - client/src/net/**
  - server/staff/**
  - server/admin/**
  - server/adminworld/**
  - schema/proto/**
  - schema/gen/**
provides: []
depends_on:
  - on: instances-core
    kind: behavior
    via: instances
---

# Instances: client window, admin screen and staff endpoint

## Goal

Build the parts of proposal 0003 that people see: the client's memorial dungeon window from `InstanceStatus` (queue position, both countdowns, the destroy button when `can_destroy`) and the notice of `InstanceEnded`; the instances list on the admin World screen; `POST /instances/{id}/destroy` under the new staff permission `world.instances`, held by admin and game_master, with an audit row.

## Context

Split from instances-core on 2026-10-27, because the proposal showed instances-core was more than one L node. The protocol messages are in the frozen `instances` contract; build against a stub registry until instances-core lands.

- Source: docs/proposals/0003-instances.md

## Acceptance

- [ ] The client shows the window and its countdowns, and the destroy command ends an instance (browser check or client test).
- [ ] Staff with `world.instances` see running instances and end one; the audit log has the row; others get 403.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Client window plus admin screen and one endpoint; known patterns from decisions 0028 and 0031.
