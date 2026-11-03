---
id: FETCH-0002
title: "gh bridge: notifications"
status: done
kind: feature
priority: p0
size: s
depends_on: [FETCH-0001]
touches:
  - Packages/AcmeCore/Sources/AcmeCore/GitHub/**
  - Packages/AcmeCore/Tests/AcmeCoreTests/GitHub/**
context:
  - docs/product.md#scoring
  - specs/0001-app-shell.md
human_review: optional
---

# FETCH-0002: gh bridge: notifications

## Why

Notifications from acme/widgets reach the app.

## What

Runs `gh api notifications` and reads the JSON.

## Acceptance criteria

- [x] AC-1: The sample notification reads like a person wrote it (human: read the sample)
- [x] AC-2: Parses the fixture (test: GHBridgeTests.testParse)
- [x] AC-3: The error for a missing gh is in Fetch's voice (human: read the message)

## Verification

`make test-core`

## Out of scope

- Posting anything.

## Decisions

- 2026-10-01: Chose `gh api` over the REST client because it reuses the login.

## Notes for the agent

The fixture is `acme/widgets`.
