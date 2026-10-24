# Character opt-in (draft)

Sparring notes. Not implemented yet.

## Goal

Discovering a character must not silently enroll it in GutSaga. Banks and alts stay out until the player (or MCP) turns them on. First character is **not** default-yes.

## Model

- **Seen** — addon reported `CHARACTER`; row exists, linked to the Smia player.
- **Participating** — explicit flag (e.g. `participating` / `gutsaga_enabled`), default `false`.
- Sync, progress, completions, Discord, and tray “active” list only use participating characters.
- Non-participating toons: upsert/last_seen still update; quest complete should no-op or 403 clearly.

## Auth / identity (unchanged)

- Keep **Smia** as companion login and membership gate (invite to the hub → play together).
- Skip Battle.net OAuth; roster comes from the game, not Blizzard’s API.
- Optional later: Smia MCP “set up companion” claim/deep link to skip the browser dance on a new machine.

## UX

- **Prompt in the addon**, not the companion (no native dialog).
- Fire after character login / when `CHARACTER` is known and `participating == false` and the player has not already chosen Nei (or snoozed).
- Do not re-prompt on every `/reload`.
- Copy roughly: “Bruke Magní (Celebras) i GutSaga?” → Ja / Nei / Ikke nå.
- Companion tray can still list known vs active and deep-link mental model (“ingen aktive”), but consent lives in-game.
- Empty state is fine: no participating chars → no saga entries until they opt in.

## API (companion + MCP)

Player session:

- `GET /v1/characters` — known toons with `participating`, level, class, realm, `last_seen_at`
- `PATCH /v1/characters/{id}` — `{ "participating": true|false }`
- Optional: bulk `PUT /v1/characters/participating`

MCP (gutsaga / wow server):

- `list_characters`
- `set_character_participating`

So Magnus can flip friends’ banks off without them hunting UI.

## Onboarding (target)

1. Invite via Smia. Call MCP `get_download_instructions` with the player's `os` (`macos` or `windows`) and `arch` (`arm64` or `amd64`). It returns the matching download and the install facts. Say those steps in the language of the conversation. Then `list_sagas` / `create_saga`. Auto-update is on by default; the tray says “Ny versjon … start på nytt” instead of restarting by itself.
2. Install companion → addon installs automatically.
3. DB empty of *their* characters until first game login.
4. Log into a toon → addon asks to join GutSaga.
5. After Ja + sync/`/reload` → party sagas from other hub members.

## Out of scope for this note

- Battle.net login or roster import.
- Companion modal (explicitly rejected in favor of addon UI).
- Changing Smia as the who-plays-together control plane.
