---
id: FETCH-0003
title: Rule engine
status: ready
kind: feature
priority: p1
size: m
depends_on: [FETCH-0002]
touches:
  - Packages/AcmeCore/Sources/AcmeCore/Rules/**
  - specs/0003-rule-engine.md
  - specs/0001-app-shell.md
  - specs/README.md
context:
  - specs/0002-gh-bridge.md#decisions
  - docs/architecture.md
human_review: required
---

# FETCH-0003: Rule engine

## Why

Most items can be settled by rules.

## What

Builds on [the shell](./0001-app-shell.md) and feeds [Needs you](0004-needs-you-screen.md).
See also `specs/0002-gh-bridge.md` and specs/0099-not-a-spec.md.

## Acceptance criteria

- [x] AC-1: Drafts score lower (test: RuleEngineTests.testDraftPenalty)
- [x] AC-2: The pile names match specs/0001-app-shell.md (test: RuleEngineTests.testPiles)
- [ ] AC-3: The piles read right to a person (human: skim the piles for acme/widgets)

## Verification

`make test-core`

## Out of scope

- Claude.
