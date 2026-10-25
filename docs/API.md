# GutSaga API

HTTP API for the GutSaga companion and, later, a Smia admin client. Storage is Postgres. The process runs migrations on startup and serves `GET /healthz`.

Player-facing text (Discord, the adventure log, the WoW addon) is Norwegian. This document is English.

## Run

Environment:

| Variable | Required | Purpose |
|---|---|---|
| `DATABASE_URL` | yes | Postgres URL. Railway's Postgres plugin sets this. |
| `ADMIN_TOKEN` | for admin routes | Bearer token for `/v1/admin/*`. Not a player key. |
| `PUBLIC_BASE_URL` | for signed covers | Public origin of this API, no trailing slash. Example: `https://gutsaga.example.com`. Admin saga reads turn it into absolute `image_signed_url` values. |
| `IMAGE_URL_SECRET` | for signed covers | HMAC secret for those URLs. `openssl rand -hex 32`. Both this and `PUBLIC_BASE_URL` must be set or the signed URL is omitted. |
| `IMAGE_URL_TTL` | no | Lifetime of a signed cover URL. Go duration, default `1h`. |
| `DISCORD_WEBHOOK_URL` | no | Incoming webhook. Saga announcements, version notes, retirements, milestone levels, and completions are posted as username `GutSaga`. There is no second Discord variable. Discord attaches cover and screenshot bytes itself; it does not use the signed URL. |
| `PORT` | no | Listen port. Railway sets this. Default `8080`. |
| `SMIA_AUTH_MODE` | no | `smia` (default) calls `https://smia.tools/wow/mcp`. `dev` accepts `SMIA_DEV_TOKEN` instead. |
| `SMIA_MCP_URL` | no | Override the MCP resource URL. |

```bash
export DATABASE_URL=postgres://gutsaga:gutsaga@127.0.0.1:5432/gutsaga?sslmode=disable
export ADMIN_TOKEN=$(openssl rand -hex 32)
export PUBLIC_BASE_URL=http://127.0.0.1:8080
export IMAGE_URL_SECRET=$(openssl rand -hex 32)
export DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
go run ./api
```

Mint a player key (printed once):

```bash
go run ./api mint --battletag 'Magnus#2231' --name Magnus
go run ./api mint --battletag 'Magnus#2231' --name Magnus --rotate
```

The same thing over HTTP is `POST /v1/admin/players`. On Railway, after deploy:

```bash
curl -s -X POST "$API/v1/admin/players" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"battletag":"Magnus#2231","display_name":"Magnus"}'
```

The response contains `api_key` (`gsk_…`). Store it in that player's companion config. It is not shown again. Rotate with `POST /v1/admin/players/{id}/rotate-key`.

## Auth

Players sign in with Smia. The companion does OAuth (authorization code + PKCE, public client, loopback redirect) and sends the Smia access token once:

`POST /v1/auth/session` with `Authorization: Bearer <smia access token>`.

The API calls the `wow` hub MCP (`https://smia.tools/wow/mcp`), tool `check_access`, and trusts only that response. A JSON body is ignored. Non-members get `403 not_a_member`. A member gets a GutSaga session (`gss_…`, 30 days). Later companion calls use that session. The Smia token is cached by hash for about eight minutes and is not stored.

`SMIA_AUTH_MODE=dev` plus `SMIA_DEV_TOKEN` skips the network call for local debugging.

- Companion routes (`/v1/sync`, `/v1/complete`, `DELETE /v1/progress`, `/v1/level`, `PUT /v1/characters`): `Authorization: Bearer <session or legacy gsk_ key>`.
- Admin routes (`/v1/admin/*`): `Authorization: Bearer <ADMIN_TOKEN>`.
- `GET /healthz` is open. It pings Postgres and returns `{"ok":true}`.

Legacy API keys (`gsk_…` from `mint`) still work. They are stored as SHA-256 hashes.

### `PUT /v1/characters`

The companion upserts the character the addon reported. BattleTag is taken from the game, not typed in.

```json
{"name":"Magni","realm":"Stormwind","level":12,"race":"Dwarf","class":"Warrior","battletag":"Magnus#2231"}
```

## Objectives

`objectives` is a JSON array of steps. A saga completes the first time every step is satisfied on the same check, and that completion stays. Smia MCP `create_saga` / `update_saga` pass this array through to `POST` / `PATCH /v1/admin/sagas` unchanged.

Every step has `id` (`[A-Za-z0-9_-]{1,40}`, unique across the saga including sequence beats), `type`, and `text` (the quest-log line, max 300). At most 20 top-level steps and 40 nodes. Leave `objectives_ordered` false. Order inside one trick is a `sequence` step.

`memory` is `visit` or `hold`:

| `memory` | Checkbox |
|---|---|
| `visit` | Once true, stays ticked. Default for `go`. Always used for `talk`, `emote`, `kill`, and `sequence`. |
| `hold` | Ticked only while it is true now. Default for `item`. Also valid on `go` ("be here now"). |

| `type` | Fields |
|---|---|
| `go` | `zone`, `subzone` (client language; `\|` or `;` for alternatives), `map_id` (UiMapID; Elwynn Forest is 37), `x`, `y` (0–1), `radius` (default 0.02). Optional `memory`. |
| `talk` | `npc` and/or `object`. `npc` requires opening dialogue (gossip, quest, merchant, or trainer) — targeting alone does not count. `object` matches the current target. Names follow the client language. Not GUIDs. Always `visit`. |
| `item` | `item_id` (required), optional `item_name`, `count` (default 1). Optional `memory`. |
| `emote` | `emote` token such as `SALUTE` or `/salute`. Optional `npc` (target at emote time). Optional place fields, same as `go`, captured when the emote fires. |
| `kill` | `npc` (creature name, not a GUID), `count` (default 1). A secret combat-log name does not count. Optional `group` (below). |
| `sequence` | `within_sec` (1–600) and `beats` (2–6). Each beat is a `go`, `talk`, `emote`, or `kill` with its own `id` and `text`. One checkbox. The window starts when the first beat lands, on the computer clock. A later beat after the window resets the sequence. Repeating the first emote, talk, or kill restarts the window. |

Same-moment constraints are fields on one step: "salute Wilhelm" is one emote with `npc`, not two steps. Two steps are both required. Cross-verb "either" is not a type.

`location`, `interact`, and `collect` are older names for `go`, `talk`, and `item`. `all_of` and `any_of` still evaluate (`objectives`, at least two children, nesting at most 4) but new sagas should not use them. Inside `all_of`, children stay live until the composite has been true, then it latches.

### Group gate on a kill

```json
"group": { "level": 17, "size": 5, "instance": "The Deadmines|Deadmines" }
```

This is not the saga-level turn-in rule. On a kill step:

- `level` is exact. Every party member must be that level. A 16 or an 18 fails the step.
- `size`, when set, is the exact roster size including the player. When omitted, the whole party must qualify and there must be at least two people.
- `instance` matches `GetInstanceInfo()` or the zone, with `\|` / `;` alternatives.
- The death does not latch by itself. Each member's addon attests from inside the instance (`H` on addon channel `PARTY`/`RAID`) within 15 seconds. Someone who is not in the dungeon never attests, so the kill does not count. The addon cannot see that a member is standing in the boss room.

A saga-level `group` is still accepted for older sagas. It means "at least `size` members of `min_level` or higher" at turn-in, and extra members are fine. New "everyone, exactly this level" sagas put `group` on the kill instead.

### Time window (`when`)

Optional on any objective and on the saga. Both a time-of-day window and an absolute range may be set; they are AND. A window may cross midnight (`22:00`–`02:00`). `starts_at` / `ends_at` are RFC3339.

`clock` is `local` (default), `game`, or `realm`:

| `clock` | What the addon reads | Use it for |
|---|---|---|
| `local` | `time()` and `date("*t")` — the computer's clock | Real local time of day, and absolute dates. This is the reliable default on Classic Era, Mists, TBC, and Forever. |
| `game` | `GetGameTime()` — the in-game day/night clock (the clock UI). No calendar. | "At night in Azeroth". `starts_at` / `ends_at` are rejected. |
| `realm` | `GetServerTime()` formatted with `date("!*t", serverTime)` as **UTC** | Realm unix time. Not the player's timezone and not the in-game clock. If `GetServerTime` fails, the window stays closed. |

The addon does not fall back from one clock to another. Forever may return a secret number from `GetGameTime` during combat; the last plain reading is kept. Absolute dates are not checked on the server, because the API is not the player's clock.

While a saga `when` is closed, nothing new can complete. Objectives already completed stay completed. The saga turns in the next time the window is open and every objective is done.

### Group sagas (`group`)

Optional on the saga, checked when the objectives are all done:

```json
"group": { "size": 4, "min_level": 17, "instance": "The Deadmines|Deadmines" }
```

- `size` (1–40) is how many party or raid members must be present, including the player. Members below `min_level` do not count. Extra members are fine.
- `instance` matches `GetInstanceInfo()`'s name or the zone (`GetZoneText`), with the same `\|` / `;` alternatives as NPC names. Omit it for "anywhere, as long as they are grouped".
- `party` or `assigned_character_ids` still decide who can see the saga. Set `party: true` for a Deadmines run the whole group should see.

The addon reads `GetNumGroupMembers` (or `GetNumPartyMembers` / `GetNumRaidMembers` on older calls), `UnitName`, and `UnitLevel`. On Forever those can be secret in combat, so the last plain roster is kept from the last moment names and levels were readable (walking in, out of combat). A member whose level was never read does not count.

When the finisher's objectives are done and the group qualifies, the addon:

1. Writes the roster into SavedVariables (`group_members`, `instance`). The companion sends it on `POST /v1/complete`.
2. Sends `C_ChatInfo.SendAddonMessage` (prefix `GutSaga`, channel `PARTY` or `RAID`) **even inside an instance**. Peers who have the addon, have the saga, and locally see a qualifying group mark the saga complete too. They do not re-check the finisher's objectives.

The API records one progress row per known character (name + realm already registered). Repeating the call is idempotent: the first completion wins, and a later call only fills in group members who were missing. Characters the API has never seen are skipped until their own companion syncs. Discord is posted for the submitting character, same as a solo saga. Each credited character gets an adventure-log line.

A group saga cannot be completed without a roster that meets `size`, `min_level`, and `instance`. The API trusts the addon's instance string; it cannot see the dungeon itself.

### Examples

```json
"objectives": [
  {"id": "inn", "type": "go", "memory": "visit", "text": "Vær innom Goldshire", "map_id": 37, "x": 0.438, "y": 0.658, "radius": 0.05},
  {"id": "here", "type": "go", "memory": "hold", "text": "Stå i Goldshire", "zone": "Elwynn Forest", "subzone": "Goldshire"},
  {"id": "talk", "type": "talk", "text": "Snakk med Innkeeper Farley", "npc": "Innkeeper Farley"},
  {"id": "water", "type": "item", "text": "Ha kildevann", "item_id": 159, "count": 1},
  {"id": "salute", "type": "emote", "text": "Hils på vakten", "emote": "SALUTE", "npc": "Stormwind Guard",
    "when": {"clock": "local", "start": "22:00", "end": "02:00"}},
  {"id": "trick", "type": "sequence", "text": "Klapp, deretter en frekk gest innen 30 sekunder", "within_sec": 30, "beats": [
    {"id": "clap", "type": "emote", "text": "Klapp for Brother Wilhelm", "emote": "APPLAUD", "npc": "Brother Wilhelm"},
    {"id": "rude", "type": "emote", "text": "Vær frekk mot Innkeeper Farley", "emote": "RUDE", "npc": "Innkeeper Farley"}
  ]}
]
```

Edwin VanCleef with the whole party at exactly level 17, inside the dungeon when he dies:

```json
{
  "title": "Deadmines",
  "party": true,
  "min_level": 17,
  "objectives": [
    {"id": "vancleef", "type": "kill", "text": "Drep Edwin VanCleef sammen", "npc": "Edwin VanCleef",
      "group": {"level": 17, "instance": "The Deadmines|Deadmines"}}
  ]
}
```

`go run ./api seed` writes three Elwynn test quests (see the README section Lokal testing).

## Companion

Base URL is whatever the companion config calls `api_base_url` (no path prefix beyond `/v1`).

### `GET /v1/sync`

Returns three arrays. The companion merges them; the server does not hide quests the character is too low for.

```json
{
  "battletag": "Magnus#2231",
  "generated_at": "2026-10-22T14:44:27Z",
  "characters": [
    {"id": "…", "name": "Magni", "realm": "Argent Dawn", "level": 12, "race": "Dwarf", "class": "Paladin", "battletag": "Magnus#2231"}
  ],
  "sagas": [
    {
      "id": "…", "title": "Første vakt", "description": "…",
      "objectives": [{"id": "stay", "type": "location", "text": "Stå vakt", "zone": "Elwynn Forest"}],
      "min_level": 10, "reward": "Ære", "party": false,
      "assigned_to": [{"id": "<character id>"}], "created_by": "Magnus",
      "active": true, "version": 3,
      "requires": [],
      "chain": {"id": "goldshire", "name": "Goldshire", "order": 1, "index": 1, "total": 2}
    }
  ],
  "progress": [
    {"saga_id": "…", "character_id": "…", "saga_version": 3, "status": "completed", "completed_at": "2026-10-22T14:44:00Z", "screenshot_url": "https://…"}
  ],
  "locks": [
    {"saga_id": "…", "character_id": "…", "locked": false, "unlocked": true}
  ]
}
```

`quests` is every active quest, plus inactive quests this player already has progress on. `progress` is only this player's rows. `screenshot_url` is set only when Discord returned a public attachment URL.

`requires` lists prerequisite saga ids (all must be completed by that character). `chain` is optional display grouping; `index` and `total` are computed (`Kapittel 2 av 4`). `locks` is one row per character of this player and per quest in the payload. `locked` is false when that character has already completed the saga. A prerequisite completed on an older `saga_version` still counts. See the README section Saga-kjeder.

Merge rule, applied by the companion when it writes `GutSagaData.lua`:

- The quest is active, and either `party` is true or `assigned_to` contains the character id. `min_level` is not a hard gate — the addon colors the title by expected difficulty (WoW-style red/orange/yellow/green/gray). No `min_level` uses yellow.
- Status comes from the progress row with the same `saga_id` and `character_id`. No row means not started.
- A completed quest stays in the log even if it is later deactivated.
- A locked saga stays in the log when the character is otherwise eligible. The addon shows it greyed out.

`?battletag=` is optional and must match the key.

### `GET /v1/ws`

WebSocket upgrade. Same `Authorization: Bearer` as `/v1/sync`. A missing or unknown session is `401` and the socket is not upgraded.

Each text frame is one JSON object. It is a hint, not the saga. The companion calls `GET /v1/sync` and rewrites `GutSagaData.lua`.

```json
{"type":"saga.created","saga_id":"<uuid>","version":1}
{"type":"saga.updated","saga_id":"<uuid>","version":2}
{"type":"saga.retired","saga_id":"<uuid>","version":3}
{"type":"saga.completed","saga_id":"<uuid>","version":1}
{"type":"level.changed","player_id":"<uuid>"}
{"type":"ping"}
```

`saga.created`, `saga.updated`, `saga.retired`, and `saga.completed` go to every connected companion. `level.changed` goes only to that player. `ping` is a heartbeat, every 25 seconds, and is not a reason to sync.

The frames are produced by Postgres triggers (`LISTEN`/`NOTIFY` on `gutsaga_events`) so every API process sees them. Image-only updates do not notify. Railway notes, reconnect, and shutdown are in [realtime.md](realtime.md).

### `POST /v1/complete`

Idempotent on `(character_id, saga_id)`. The first call wins and stores `saga_version` — the version the player had loaded, which must exist in `quest_versions`. Repeating the call does not change the row, does not write another adventure-log line, and does not post to Discord again. A later quest version does not open a second completion.

Multipart (`screenshot` file) or JSON. Send the screenshot only when one exists. Do not send a local file path.

```json
{
  "character_id": "<uuid>",
  "saga_id": "<uuid>",
  "saga_version": 3,
  "battletag": "Magnus#2231",
  "level": 12,
  "completed_at": "2026-10-22T14:44:00Z",
  "screenshot_base64": "<optional>",
  "instance": "The Deadmines",
  "group_members": [
    {"name": "Magni", "realm": "Argent Dawn", "level": 18},
    {"name": "Mira", "realm": "Argent Dawn", "level": 17}
  ]
}
```

`instance` and `group_members` are required only when that quest version has a `group` rule. Multipart uploads send `group_members` as a JSON string. The response may include `group_credited`: characters other than the caller who were marked complete. `already: true` means that character had already finished.

`battletag` must match the API key, and the character must belong to that player. Otherwise `403` and `"allowed": false`.

A saga with unfinished prerequisites is rejected with `409` and `{"error":"locked","message":"Låst: fullfør … først"}`. The check uses the current `requires` list. Any completed progress row for a prerequisite counts, whatever `saga_version` it stored. Repeating a completion that already exists stays `200` / `already_completed`, even if a prerequisite was added later.

```json
{
  "allowed": true,
  "already_completed": false,
  "completed": true,
  "progress_id": "…",
  "log_written": true,
  "discord_posted": true,
  "character_name": "Magni",
  "saga_title": "Første vakt",
  "saga_version": 3
}
```

On a new completion the API:

1. Stores the image in Postgres (TGA is converted to PNG).
2. Inserts progress and an adventure-log line: `Magni fullførte «Første vakt» (versjon 3).`
3. Queues a Discord embed, username `GutSaga`. The content line is short (`**Magni** fullførte en saga!`); the embed carries the saga title, class, level, version, reward, timestamp, and screenshot (`attachment://screenshot.png`, so Discord does not need a public GutSaga URL).

The HTTP call does not wait for Discord. `discord_posted` is true when the notice was accepted onto the queue (or delivered, if a synchronous notifier is plugged in). The progress row's `discord_url` is written when Discord returns the attachment URL. A webhook error, including HTTP 429, is retried in the background (`retry_after` on 429, backoff on network errors and 5xx) and then logged. It does not change the HTTP status and does not roll back the completion. When the queue cannot accept the notice, `discord_posted` is false.

The completion body also echoes `character_class`, `character_level`, and `reward` from the version that was completed.

### `POST /v1/level`

Milestone levels (10, 20, 30, …) are queued as a short embed: content `**Magni** nådde et nytt nivå.`, embed title `Nivå 20`. Other levels only update the character and the adventure log.

```json
{"character_id": "<uuid>", "battletag": "Magnus#2231", "level": 13}
```

Updates the character. The same level again is a success with `"changed": false` and no extra log line. A change writes `Magni nådde nivå 13.`

### `DELETE /v1/progress`

Clears progress rows for the signed-in player so a local `/gutsaga wipe` or `/gutsaga reset …` is not undone by the next companion sync. Optional `?saga_id=<uuid>` limits to one saga; omit it to clear every row for that player's characters.

```json
{"ok": true, "deleted": 2, "saga_id": ""}
```

The companion reads `GutSagaDB.pending_resets` from SavedVariables (after `/reload`), calls this endpoint, then syncs again. Adventure-log lines are left in place.

## Admin

These are the endpoints a later Smia MCP server should call with `ADMIN_TOKEN`.

| Method | Path | Body / query |
|---|---|---|
| `POST` | `/v1/admin/players` | `battletag`, `display_name` → player + `api_key` |
| `GET` | `/v1/admin/players` | |
| `POST` | `/v1/admin/players/{id}/rotate-key` | returns a new `api_key` |
| `POST` | `/v1/admin/characters` | `player_id` or `battletag`, `name`, `realm`, `race`, `class`, `level`, `backstory` |
| `GET` | `/v1/admin/characters?battletag=` | |
| `GET` | `/v1/admin/characters/{id}` | |
| `PATCH` | `/v1/admin/characters/{id}` | any of name, realm, race, class, level, backstory |
| `DELETE` | `/v1/admin/characters/{id}` | `204` |
| `POST` | `/v1/admin/sagas` | title, description, objectives, optional `when`, optional `group`, optional `requires` (saga ids), optional `chain` (`id`, `name`, `order`), reward, `min_level`, `party`, `assigned_character_ids`, `active`, `created_by`. JSON or multipart. Optional cover via multipart file `image` or JSON `image_base64` (max 8 MiB; attached to the Discord announcement). Optional `announce` (default: post when the saga is active; `false` skips; `true` posts even a draft). Response includes `image_signed_url` when the saga has a cover and signing is configured. |
| `GET` | `/v1/admin/sagas?character_id=` | every quest; with `character_id`, includes `status` (`null` if not started), `eligible`, `completed_version`, `locked`, `unlocked`, `missing`. Quests that have a cover include `image_signed_url` when signing is configured. |
| `GET` | `/v1/admin/sagas/{id}` | saga plus `versions[]` snapshots (each snapshot includes `requires` and `chain`), `completed_by[]` (character, realm, class, level, when), `goals_markdown` (numbered objectives for Smia stack text), and `card` (Smia Aktivitet table: character, detail, when). Same `image_signed_url` as the list. |
| `GET` | `/v1/admin/sagas/{id}/image` | cover bytes and `Content-Type`, same as `GET /v1/sagas/{id}/image`. `404` when the saga has no image. |
| `PUT` | `/v1/admin/sagas/{id}/image` | raw or multipart `image` cover (same rules as player `/v1/sagas/{id}/image`) |
| `DELETE` | `/v1/admin/sagas/{id}/image` | clear cover |
| `PATCH` | `/v1/admin/sagas/{id}` | partial update. Text changes **bump `version` and write a snapshot.** `min_level: null` clears it. JSON or multipart; optional file field `image` (cover-only OK, no version bump); optional `when` / `group`. `requires: []` clears prerequisites. `chain: null` removes the saga from its chain. Optional `editor_email` (Smia caller): when set, only that player's sagas may be edited (`403` otherwise; Smia `update_saga` sends this from `context.run`). Optional `updated_by` (BattleTag) is the same check by tag. Optional `announce` (default true). A text, goal, reward, minimum-level, or party change posts a one-line version embed. `active: false` posts a retirement instead. Assignment-only edits stay quiet. `announce: false` skips both. Response includes `image_signed_url` when signing is configured. |
| `GET` | `/v1/public/placeholder.png` | 640×360 dark PNG (no auth). Used by Smia `get_saga` when a saga has no cover so the media card keeps a readable width (`/v1/public/empty.png` is an alias). |
| `GET` | `/v1/public/sagas/{id}/image?exp=&sig=` | no `Authorization` header. HMAC-SHA256 of `id`, a newline, and the unix `exp`, hex-encoded, keyed with `IMAGE_URL_SECRET`. `403` bad signature (checked before the database, so a guess does not reveal whether the saga exists). `403` when `exp` is in the past. `404` when the signature is valid and there is no image. The body is only the image bytes. |
| `GET` | `/v1/admin/log?battletag=&character_id=&limit=` | newest first, default 50 |
| `POST` | `/v1/admin/log` | `type`, `title`, `body`, optional `battletag` / `player_id` / `character_id` / `saga_id` |
| `DELETE` | `/v1/admin/progress` | `character_id` and/or `saga_id` (at least one) |

Create quest starts at version 1 and stores that snapshot. Realm must match what the Forever client reports (`GetRealmName()`), including spaces.

Errors:

```json
{"error": "invalid", "message": "…"}
```

`400` invalid, `401` missing/wrong token, `403` wrong owner, `404` missing, `409` conflict (duplicate BattleTag or character name+realm).

## Data

| Table | Notes |
|---|---|
| `players` | BattleTag, display name, API key hash |
| `characters` | name, realm, race, class, level, backstory, owner |
| `quests` | current text, `min_level`, party flag, active, **version**, `requires`, optional chain |
| `quest_versions` | immutable snapshot per version, including assignments, `requires`, and chain |
| `progress` | one row per character+quest, the version they completed, screenshot id, Discord URL |
| `screenshots` | image bytes |
| `adventure_log` | Norwegian text |

`schema_migrations` is applied automatically. See `api/migrations/`.

The machine-readable contract is also in [`openapi.yaml`](openapi.yaml).

## Railway

Connect the GitHub repo to a Railway project. The service root directory must be the **repository root** (the Dockerfile context is the whole module). `railway.json` builds `api/Dockerfile` and checks `/healthz`. Do **not** deploy with `docker-compose.yml` — that file is local-only and ships `dev-admin-token` / `dev-image-url-secret`. The API refuses those values at startup unless `GUTSAGA_ALLOW_DEV_SECRETS=1`. Add the Postgres plugin and set `DATABASE_URL`, `DISCORD_WEBHOOK_URL`, and `ADMIN_TOKEN` (`openssl rand -hex 32`). For chat clients that render a saga cover as a markdown image, also set `PUBLIC_BASE_URL` to the public origin of the API (no trailing slash) and `IMAGE_URL_SECRET` (`openssl rand -hex 32`). `IMAGE_URL_TTL` is optional and defaults to `1h`. Pushes to `main` deploy through Railway's GitHub integration.

WebSockets need no extra Railway setting. The public proxy upgrades `GET /v1/ws` and does not apply the HTTP idle timeout to that socket. Leave `ReadTimeout` and `WriteTimeout` unset on the Go server (they are). Details, heartbeats, and deploys: [realtime.md](realtime.md).
