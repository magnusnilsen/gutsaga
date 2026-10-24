# Saga authoring API

Player-authenticated endpoints for creating and maintaining your own sagas.
Use the companion session API key (`Authorization: Bearer …` from `POST /v1/auth/session`).

Admin can still use `/v1/admin/sagas` (any creator). When the patch body includes `editor_email` (Smia caller) or `updated_by` (BattleTag), the API enforces the same ownership rule as players — only that author may edit (Smia MCP `update_saga` sends `editor_email` from `context.run`). Players use `/v1/sagas` and may only update sagas they own (`created_by_player_id`).

## Goal types

Each saga needs `objectives` (1–20 steps). The saga completes the first time every step is satisfied together, and that completion stays. Leave `objectives_ordered` false.

| Field | Required | Notes |
| --- | --- | --- |
| `id` | yes | `[A-Za-z0-9_-]{1,40}`, unique across steps and sequence beats |
| `type` | yes | `go`, `talk`, `item`, `kill`, `emote`, or `sequence` |
| `text` | yes | player-facing line (max 300) |
| `memory` | no | `visit` (latches) or `hold` (only while true). Default `visit` on `go`, `hold` on `item`. Talk, emote, kill, and sequence are always `visit`. |

### `go`

Be at a place. `memory: "visit"` is a stop on a tour. `memory: "hold"` is "stand here now" and clears when you leave.

- `zone`, `subzone` — name match (`\|` / `;` for alternatives)
- `map_id`, `x`, `y` — map fractions `0..1`; `radius` defaults to `0.02`

Needs at least one of zone, subzone, map_id, or x/y.

### `talk`

Open dialogue with an NPC, or select a world object. `npc` requires gossip / quest / merchant / trainer (`UnitName("npc")`) — targeting alone does not count. `object` matches the current target. Always `visit`: once you have talked, it stays ticked.

### `item`

Have N of an item in bags. `item_id` required. `count` defaults to 1. `memory: "hold"` (the default) clears if the item leaves the bags. `memory: "visit"` latches once you had it.

### `emote`

A built-in emote. `SALUTE`, `salute`, or `/salute` are stored as `SALUTE`. Optional `npc` is the target. Optional place fields use the position captured when the emote fires. Custom `/e hello` does not count.

### `kill`

Defeat N creatures by name (`npc`, `count` default 1). A secret combat-log name does not count.

Optional `group` makes the kill a together-step. `level` is exact for every party member. `size`, when set, is the exact roster size. `instance` matches the dungeon name or zone. The kill latches only when every member's addon has attested from inside that instance within 15 seconds of the death.

```json
{ "id": "vancleef", "type": "kill", "text": "Drep Edwin VanCleef sammen", "npc": "Edwin VanCleef",
  "group": { "level": 17, "instance": "The Deadmines|Deadmines" } }
```

### `sequence`

One checkbox with ordered beats and a single deadline.

- `within_sec` — 1–600, measured on the computer clock from the first beat
- `beats` — 2 to 6 steps, each `go`, `talk`, `emote`, or `kill`

A beat after the window resets the sequence. Repeating the first emote, talk, or kill restarts the clock. The other steps in the saga are not ordered.

```json
{ "id": "trick", "type": "sequence", "text": "Klapp, deretter /rude innen 30 sekunder", "within_sec": 30,
  "beats": [
    { "id": "clap", "type": "emote", "text": "Klapp for Wilhelm", "emote": "APPLAUD", "npc": "Brother Wilhelm" },
    { "id": "rude", "type": "emote", "text": "Vær frekk mot Farley", "emote": "RUDE", "npc": "Innkeeper Farley" }
  ] }
```

`location`, `interact`, and `collect` still mean `go`, `talk`, and `item`. `all_of` / `any_of` still evaluate, and new sagas should not send them.

### Time window

Optional `when` on an objective or on the saga. `clock` defaults to `local`.

| `clock` | Client API | Meaning |
|---|---|---|
| `local` | `time()`, `date("*t")` | Computer local time. Default. Works on Classic and Forever. |
| `game` | `GetGameTime()` | In-game day/night clock. No date. `starts_at` / `ends_at` are rejected. |
| `realm` | `GetServerTime()` + `date("!*t")` | Realm unix time as UTC. |

```json
"when": { "clock": "local", "start": "22:00", "end": "02:00", "starts_at": "2026-11-01T00:00:00Z", "ends_at": "2026-11-30T23:00:00Z" }
```

`start` and `end` are `HH:MM` and may cross midnight. They are AND with `starts_at` / `ends_at` when both are set. A closed saga window blocks new completions; objectives already done stay done.

### Group on the saga

Older turn-in rule. Prefer `group` on a kill step when every member must be an exact level and present for the kill.

```json
"group": { "size": 4, "min_level": 17, "instance": "The Deadmines" }
```

`size` members (including you) of at least `min_level` must be present. Extra members are fine. People below `min_level` do not count. `instance` matches `GetInstanceInfo` or the zone.

Completion is stored once per character. The finisher's companion sends the roster; the API also credits every other character it already knows with that name and realm. Peers with the addon get an addon message (`C_ChatInfo.SendAddonMessage` on `PARTY`/`RAID`, including inside the instance) and write their own completion. A second submit does not duplicate rows.

## Goal order

`objectives_ordered` defaults to **`false`**. Leave it false. A timed order is a `sequence` step, not a second saga and not this flag.

## Endpoints

### `POST /v1/sagas`

Create a saga owned by you. Body:

```json
{
  "title": "En kveld i Goldshire",
  "description": "…",
  "objectives": [
    { "id": "inn", "type": "go", "text": "Stå ved vertshuset", "map_id": 37, "x": 0.44, "y": 0.66 },
    { "id": "farley", "type": "talk", "text": "Snakk med Farley", "npc": "Innkeeper Farley" },
    { "id": "water", "type": "item", "text": "Ha vann", "item_id": 159, "count": 1 }
  ],
  "objectives_ordered": false,
  "when": { "clock": "local", "start": "22:00", "end": "02:00" },
  "reward": "",
  "min_level": 1,
  "party": false,
  "active": true,
  "assigned_character_ids": []
}
```

Empty `assigned_character_ids` with `party: true` offers the saga to everyone eligible by level.

Optional chain fields, same shape as the admin API (see the README section Saga-kjeder):

```json
"requires": ["<prerequisite saga id>"],
"chain": {"id": "goldshire", "name": "Goldshire", "order": 2}
```

All ids in `requires` must already exist. The character must complete every one of them, on any version, before this saga accepts a new completion. `chain` only groups the log (`Kapittel N av M`).

### `GET /v1/sagas`

Lists sagas you created.

### `GET /v1/sagas/{id}`

Your saga plus version history.

### `PATCH /v1/sagas/{id}`

Update fields (same shape as create, all optional). Only the owner may patch. Cannot change `created_by`.

### Cover image

- `PUT /v1/sagas/{id}/image` — raw body or multipart field `image` (max 8 MiB; stored as PNG)
- `GET /v1/sagas/{id}/image` — download (any signed-in player; used by the companion)
- `DELETE /v1/sagas/{id}/image` — clear
- Admin (Smia MCP): optional `image` file on `POST`/`PATCH` `/v1/admin/sagas`, or `PUT`/`DELETE` `/v1/admin/sagas/{id}/image`

When present, `has_image` is true and `image_url` points at the GET path. The companion converts the cover to TGA and writes `Interface/AddOns/GutSaga/media/<saga_id>.tga` on sync (Classic Era cannot load loose PNGs as addon textures). The addon shows it under the description, above goals.

The player GET stays session-authenticated. Smia, which only has the admin token, fetches the same bytes at `GET /v1/admin/sagas/{id}/image`. Admin quest list and admin quest get also include `image_signed_url` (absolute, expiring, no auth header) when `PUBLIC_BASE_URL` and `IMAGE_URL_SECRET` are set. That URL is `GET /v1/public/sagas/{id}/image` and returns only the image. See [API.md](API.md).

## After create / update

The companion syncs into `GutSagaData.lua`. Players need `/reload` in-game to see new or changed sagas.
