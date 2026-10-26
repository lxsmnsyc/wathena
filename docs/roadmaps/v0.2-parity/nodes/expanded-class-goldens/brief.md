---
id: expanded-class-goldens
title: Skill damage goldens for Gunslinger, Ninja, Taekwon and Soul Linker
status: todo
priority: should
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/battle/testdata/**
  - reference/rathena/renewal/**
  - reference/rathena/pre-renewal/**
shared:
  - server/zone/skillgolden_test.go
  - server/zone/skills_*.go
  - server/skill/**
  - server/battle/**
  - server/README.md
provides: []
depends_on:
  - on: extract-sync
    kind: hard
    why: new recordings need the fork's recorder in sync
---

# Skill damage goldens for Gunslinger, Ninja, Taekwon and Soul Linker

## Goal

Add Gunslinger, Ninja, Taekwon, Star Gladiator and Soul Linker skills to the skill damage and casting goldens in both modes, and fix the differences they show.

## Context

The skill damage tests leave these classes out today. They are within the v0.1.0 class scope. New recordings need the fork's recorder (`extract-sync`).

- Source: parity survey section 4

## Acceptance

- [ ] The goldens include every skill of these classes; parity numbers in server/README.md are updated.

## Sizing

New recordings plus fixes; medium uncertainty in how many differences appear.
