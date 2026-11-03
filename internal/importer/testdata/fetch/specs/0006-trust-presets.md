---
id: FETCH-0006
title: Trust presets
status: blocked
kind: bug
priority: p1
size: s
depends_on: [FETCH-0004, FETCH-0005]
touches:
  - App/Sources/Features/Profiles/**
context:
  - specs/0005-profiles.md
human_review: required
---

# FETCH-0006: Trust presets

## Why

New repos start at a sensible trust level.

## What

A preset per profile. Profiles come from 0005-profiles.md.

## Acceptance criteria

- [ ] AC-1: A new repo gets its profile's preset (test: TrustPresetTests.testNewRepo)

## Verification

`make test-core`

## Out of scope

- Changing presets.

## Open questions

- Waiting on the profiles spike. Proposed: start once FETCH-0005 is decided.
