---
id: FETCH-0004
title: Needs you screen
status: ready
kind: chore
priority: p1
size: m
depends_on: [FETCH-0003]
touches:
  - Packages/AcmeCore/**
  - App/Sources/Features/NeedsYou/**
context: []
human_review: required
---

# FETCH-0004: Needs you screen

## Why

One screen shows what needs you.

## What

A list.

## Acceptance criteria

- [ ] AC-1: Shows the fixture items (test: NeedsYouTests.testItems)
- [ ] AC-2: Looks like the board (human: compare with the board)

## Verification

`make check`

## Design

The board is `needs-you`.

## Out of scope

- Sorting.
