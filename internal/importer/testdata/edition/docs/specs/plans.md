# Plans and access

- **Status:** Building. Step 1 done; step 3 waits for the paid membership
- **Area:** shared (app shell, Settings)
- **Decisions:** ADR 0010
- **Depends on:** `write-and-send.md` step 1 for step 2

## Goal

People try everything for 14 days, then pay.

## Screens

| Screen | Board | Notes |
|---|---|---|
| Subscribe | Subscribe | |

## Behavior

Plans
- **S1.** Yearly and Monthly are in one group.
- **S2.** Prices come from StoreKit, as [the outbox](write-and-send.md) does for retries.

Access
- **S3.** The app is usable while any subscription is active.

## Data

See docs/specs/write-and-send.md for the outbox.

## Out of scope

A free tier.

## Open questions

Answered
- **Product IDs:** final once created.

Still open
- **When the trial starts:** Proposed: after the first edition.

## Steps

1. [x] Kit `Access` with tests (S1–S2).
   - **After:** nothing · **Touches:** EditionKit/Sources/EditionKit/Access, EditionKit/Tests/EditionKitTests/AccessTests.swift
2. [ ] Gate sync on access in the shell (S2, S3). It also adds
   a Settings line.
   - **After:** 1, write-and-send 1 · **Touches:** Edition, EditionKit/Sources/EditionKit/Gate
3. [ ] Sandbox family test.
   - **After:** 2 · **Touches:** Edition · **Blocked:** needs the paid membership

## Changes since accepted

Filled in by implementing PRs.
