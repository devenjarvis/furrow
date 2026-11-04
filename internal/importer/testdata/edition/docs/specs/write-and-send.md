# Write and send

- **Status:** Shipped
- **Area:** Mail
- **Design:** boards "Write"
- **Decisions:** none needed
- **Depends on:** nothing

## Goal

People write and send from the desk the mail came to.

## Screens

| Screen | Board | Notes |
|---|---|---|
| Write | Write | |

## Behavior

- **W1.** A reply comes from the address the mail was sent to.
- **W2.** A message that can't be sent waits in the outbox
  and is sent once the desk is back online.

## Data

- Kit: `Draft`.

## Out of scope

Send later; see `set-aside.md`.

## Open questions

Answered
- **Signatures:** one per desk.

## Steps

1. [x] Kit drafts and the outbox (W1, W2).
   - **After:** nothing · **Touches:** EditionKit/Sources/EditionKit/Mail

## Changes since accepted

Step 1:
- The outbox is a table, not a file.
