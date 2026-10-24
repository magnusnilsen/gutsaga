# Ideas (parked)

Features that exist or are sketched, but are not in the current “make the basics solid” focus.

## Saga chains (`requires` + `chain`)

**Status:** Implemented in API, sync, addon lock UI, and MCP fields. Premature to lean on for playtests.

**Why parked:** Onboarding, emotes, Discord, character opt-in, and Classic reliability matter more. Chains add authoring and QA surface before the happy path is boring.

**When to pick up:** After emote/location/interact flows and companion identity feel stable; then run the chain test plan (A unlocks B over WS).

See README section *Saga-kjeder* and [API.md](API.md).

## Character opt-in

**Status:** Design only — [character-opt-in.md](character-opt-in.md).

Discover ≠ enroll; addon prompt; MCP toggles. Do this before inventing more placeholder characters in local DB.

## Timed sequences between objectives

**Status:** Not built. `when` is clock windows (time of day / absolute range), not “within N seconds of the previous goal.”

**Wanted example:** clap for Wilhelm, then punch (emote) the innkeeper within 30 seconds.

**Sketch:** on objective complete, store `completed_at` unix; next objective’s gate is `prev.completed_at + window_sec`. Fail/reset if late. Distinct from `objectives_ordered` (order only, no deadline).

## Live top-level location ticks

**Status:** Mostly already true for `location` / `interact` / `collect` (re-evaluated each tick). Inside `all_of`, children stay live until the composite completes, then the composite sticks.

**Product note:** “Stand in Goldshire” ticking on enter and clearing on leave is the live model — good UX for presence goals. Sticky is better for one-shot emotes/kills (what we just hardened for emotes).
