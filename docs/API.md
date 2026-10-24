# GutSaga API

HTTP API for the GutSaga companion and, later, a Smia admin client. Storage is Postgres. The process runs migrations on startup and serves `GET /healthz`.

Player-facing text (Discord, the adventure log, the WoW addon) is Norwegian. This document is English.

## Run

Environment:

| Variable | Required | Purpose |
|---|---|---|
| `DATABASE_URL` | yes | Postgres URL. Railway's Postgres plugin sets this. |
| `ADMIN_TOKEN` | for admin routes | Bearer token for `/v1/admin/*`. Not a player key. |
| `PUBLIC_BASE_URL` | for signed covers | Public origin of this API, no trailing slash. Example: `https://gutsaga-production.up.railway.app`. Admin quest reads turn it into absolute `image_signed_url` values. |
| `IMAGE_URL_SECRET` | for signed covers | HMAC secret for those URLs. `openssl rand -hex 32`. Both this and `PUBLIC_BASE_URL` must be set or the signed URL is omitted. |
| `IMAGE_URL_TTL` | no | Lifetime of a signed cover URL. Go duration, default `1h`. |
| `DISCORD_WEBHOOK_URL` | no | Incoming webhook. New completions are posted as username `GutSaga`. |
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

- Companion routes (`/v1/sync`, `/v1/complete`, `/v1/level`, `PUT /v1/characters`): `Authorization: Bearer <session or legacy gsk_ key>`.
- Admin routes (`/v1/admin/*`): `Authorization: Bearer <ADMIN_TOKEN>`.
- `GET /healthz` is open. It pings Postgres and returns `{"ok":true}`.

Legacy API keys (`gsk_…` from `mint`) still work. They are stored as SHA-256 hashes.

### `PUT /v1/characters`

The companion upserts the character the addon reported. BattleTag is taken from the game, not typed in.

```json
{"name":"Magni","realm":"Stormwind","level":12,"race":"Dwarf","class":"Warrior","battletag":"Magnus#2231"}
```

## Objectives

`objectives` is a JSON array, checked in order by the addon. All of an objective's constraints are AND. `text` is the Norwegian line in the quest log. Author quests with this shape:

| `type` | Fields |
|---|---|
| `location` | `zone`, `subzone` (client language), `map_id` (UiMapID, Elwynn Forest is 37 on retail and Classic), `x`, `y` (0–1), `radius` |
| `interact` | `npc` and/or `object`. Matched to `UnitName`. Names follow the client language. |
| `collect` | `item_id`, optional `item_name`, `count` (default 1) |
| `kill` | `npc`, `count`. Experimental. A secret combat-log name does not count. |

```json
"objectives": [
  {"id": "inn", "type": "location", "text": "Stå i Goldshire", "map_id": 37, "x": 0.438, "y": 0.658, "radius": 0.05},
  {"id": "talk", "type": "interact", "text": "Snakk med Innkeeper Farley", "npc": "Innkeeper Farley"},
  {"id": "water", "type": "collect", "text": "Ha kildevann", "item_id": 159, "count": 1}
]
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
  "quests": [
    {
      "id": "…", "title": "Første vakt", "description": "…",
      "objectives": [{"id": "stay", "type": "location", "text": "Stå vakt", "zone": "Elwynn Forest"}],
      "min_level": 10, "reward": "Ære", "party_quest": false,
      "assigned_to": [{"id": "<character id>"}], "created_by": "Magnus",
      "active": true, "version": 3
    }
  ],
  "progress": [
    {"quest_id": "…", "character_id": "…", "quest_version": 3, "status": "completed", "completed_at": "2026-10-22T14:44:00Z", "screenshot_url": "https://…"}
  ]
}
```

`quests` is every active quest, plus inactive quests this player already has progress on. `progress` is only this player's rows. `screenshot_url` is set only when Discord returned a public attachment URL.

Merge rule, applied by the companion when it writes `GutSagaData.lua`:

- The quest is active, `min_level` is null or `<=` the character's level, and either `party_quest` is true or `assigned_to` contains the character id.
- Status comes from the progress row with the same `quest_id` and `character_id`. No row means not started.
- A completed quest stays in the log even if it is later deactivated.

`?battletag=` is optional and must match the key.

### `POST /v1/complete`

Idempotent on `(character_id, quest_id)`. The first call wins and stores `quest_version` — the version the player had loaded, which must exist in `quest_versions`. Repeating the call does not change the row, does not write another adventure-log line, and does not post to Discord again. A later quest version does not open a second completion.

Multipart (`screenshot` file) or JSON. Send the screenshot only when one exists. Do not send a local file path.

```json
{
  "character_id": "<uuid>",
  "quest_id": "<uuid>",
  "quest_version": 3,
  "battletag": "Magnus#2231",
  "level": 12,
  "completed_at": "2026-10-22T14:44:00Z",
  "screenshot_base64": "<optional>"
}
```

`battletag` must match the API key, and the character must belong to that player. Otherwise `403` and `"allowed": false`.

```json
{
  "allowed": true,
  "already_completed": false,
  "completed": true,
  "progress_id": "…",
  "log_written": true,
  "discord_posted": true,
  "character_name": "Magni",
  "quest_title": "Første vakt",
  "quest_version": 3
}
```

On a new completion the API:

1. Stores the image in Postgres (TGA is converted to PNG).
2. Inserts progress and an adventure-log line: `Magni fullførte «Første vakt» (versjon 3).`
3. Posts to Discord, username `GutSaga`, content `**Magni** fullførte *Første vakt*!`, with the file attached when there is one.

Discord failure does not roll back the completion. `discord_posted` is false and the error is in the API log.

### `POST /v1/level`

```json
{"character_id": "<uuid>", "battletag": "Magnus#2231", "level": 13}
```

Updates the character. The same level again is a success with `"changed": false` and no extra log line. A change writes `Magni nådde nivå 13.`

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
| `POST` | `/v1/admin/quests` | title, description, objectives, reward, `min_level`, `party_quest`, `assigned_character_ids`, `active`, `created_by` |
| `GET` | `/v1/admin/quests?character_id=` | every quest; with `character_id`, includes `status` (`null` if not started), `eligible`, `completed_version`. Quests that have a cover include `image_signed_url` when signing is configured. |
| `GET` | `/v1/admin/quests/{id}` | quest plus `versions[]` snapshots. Same `image_signed_url` as the list. |
| `GET` | `/v1/admin/quests/{id}/image` | cover bytes and `Content-Type`, same as `GET /v1/sagas/{id}/image`. `404` when the saga has no image. |
| `PATCH` | `/v1/admin/quests/{id}` | partial update. **Always bumps `version` and writes a snapshot.** `min_level: null` clears it. |
| `GET` | `/v1/public/sagas/{id}/image?exp=&sig=` | no `Authorization` header. HMAC-SHA256 of `id`, a newline, and the unix `exp`, hex-encoded, keyed with `IMAGE_URL_SECRET`. `403` bad signature (checked before the database, so a guess does not reveal whether the saga exists). `403` when `exp` is in the past. `404` when the signature is valid and there is no image. The body is only the image bytes. |
| `GET` | `/v1/admin/log?battletag=&character_id=&limit=` | newest first, default 50 |
| `POST` | `/v1/admin/log` | `type`, `title`, `body`, optional `battletag` / `player_id` / `character_id` / `quest_id` |

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
| `quests` | current text, `min_level`, party flag, active, **version** |
| `quest_versions` | immutable snapshot per version, including who it was assigned to |
| `progress` | one row per character+quest, the version they completed, screenshot id, Discord URL |
| `screenshots` | image bytes |
| `adventure_log` | Norwegian text |

`schema_migrations` is applied automatically. See `api/migrations/`.

The machine-readable contract is also in [`openapi.yaml`](openapi.yaml).

## Railway

Connect the GitHub repo to Railway project `gutsaga`. The service root directory must be the **repository root** (the Dockerfile context is the whole module). `railway.json` builds `api/Dockerfile` and checks `/healthz`. Add the Postgres plugin and set `DATABASE_URL`, `DISCORD_WEBHOOK_URL`, and `ADMIN_TOKEN`. For chat clients that render a saga cover as a markdown image, also set `PUBLIC_BASE_URL` to the public Railway origin (`https://gutsaga-production.up.railway.app`) and `IMAGE_URL_SECRET` (`openssl rand -hex 32`). `IMAGE_URL_TTL` is optional and defaults to `1h`. Pushes to `main` deploy through Railway's GitHub integration.
