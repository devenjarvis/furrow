---
id: FETCH-0001
title: App shell
status: done
kind: feature
priority: p0
size: m
depends_on: []
touches:
  - App/**
  - project.yml
context:
  - docs/product.md
human_review: required   # it's visible UI
---

# FETCH-0001: App shell

## Why

There's a window to put things in.

## What

A window with a sidebar. The GitHub bridge comes next, in specs/0002-gh-bridge.md, and the
decision behind it is docs/decisions/0012-your-line.md.

## Acceptance criteria

- [x] AC-1: The app builds (check: make build)
- [x] AC-2: The window looks right next to the board (human: compare with the board)

## Verification

```sh
make check
```

## Out of scope

- Everything else.
