# Saga authoring API

Player-authenticated endpoints for creating and maintaining your own sagas.
Use the companion session API key (`Authorization: Bearer …` from `POST /v1/auth/session`).

Admin can still use `/v1/admin/quests` (any creator). Players use `/v1/sagas` and may only update sagas they own (`created_by_player_id`).

## Goal types

Each saga needs `objectives` (1–20). Every objective has:

| Field | Required | Notes |
| --- | --- | --- |
| `id` | yes | `[A-Za-z0-9_-]{1,40}`, unique within the saga |
| `type` | yes | see below |
| `text` | yes | player-facing line (max 300) |

### `location` — place-based

Be in a zone / subzone and/or near map coordinates.

- `zone`, `subzone` — optional name match (`\|` / `;` for alternatives)
- `map_id` — optional continent/zone map id
- `x`, `y` — map fractions `0..1` (set together); `radius` defaults to `0.02`

Needs at least one of zone, subzone, map_id, or x/y.

### `interact` — selection-based

Talk to / target an NPC or world object.

- `npc` and/or `object` — at least one required (name match)

### `collect` — inventory

Have N of an item in bags.

- `item_id` — required (positive int)
- `item_name` — optional label
- `count` — defaults to `1`

### `kill` — combat (experimental)

Defeat N creatures by name. Secret combat-log names are ignored.

- `npc` — creature name (not a GUID)
- `count` — defaults to `1`

### `emote` — a built-in emote

Perform `/salute`, `/bow`, and the other built-in emotes. The addon reads the token from `DoEmote` (language-independent). `CHAT_MSG_TEXT_EMOTE` only confirms that the emote was announced; the chat text is localized and may be secret in combat on Forever.

- `emote` — required. `SALUTE`, `salute`, or `/salute` are stored as `SALUTE`
- `npc` — optional. `UnitName` of the target when the emote fires. No target, or a secret name, does not count
- location fields — optional, same as `location`, using the position captured at emote time

Custom `/e hello` is not an emote token and does not count.

### `all_of` / `any_of` — simultaneous AND, or alternatives

```json
{
  "id": "both",
  "type": "all_of",
  "text": "Stå i Goldshire med vann",
  "objectives": [
    { "id": "inn", "type": "location", "text": "Goldshire", "zone": "Elwynn Forest", "subzone": "Goldshire" },
    { "id": "water", "type": "collect", "text": "Ha vann", "item_id": 159, "count": 1 }
  ]
}
```

- `objectives` — at least two children. One nesting level is the normal case; up to 4 levels and 40 nodes total are accepted
- `all_of` is done only when every child is true in the same check. Location, item, interact, and emote are live. Kill counts still add up
- `any_of` is done when any child is true
- Child `id`s must be unique across the whole saga, not only inside the parent
- The composite stays done once it has been true

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

### Group

Optional saga field. Checked at completion, not as an objective.

```json
"group": { "size": 4, "min_level": 17, "instance": "The Deadmines" }
```

`size` members of the party or raid (including you) must be there. Each one who counts must be at least `min_level`. `instance` matches the dungeon name from `GetInstanceInfo` or the zone. Set `party_quest: true` so the whole group is offered the saga.

Completion is stored once per character. The finisher's companion sends the roster; the API also credits every other character it already knows with that name and realm. Peers with the addon get an addon message (`C_ChatInfo.SendAddonMessage` on `PARTY`/`RAID`, including inside the instance) and write their own completion. A second submit does not duplicate rows.

## Goal order

`objectives_ordered` defaults to **`false`**: any incomplete goal can complete.

Set `"objectives_ordered": true` if later goals must wait until earlier ones are done.

## Endpoints

### `POST /v1/sagas`

Create a saga owned by you. Body:

```json
{
  "title": "En kveld i Goldshire",
  "description": "…",
  "objectives": [
    { "id": "inn", "type": "location", "text": "Stå ved vertshuset", "map_id": 37, "x": 0.44, "y": 0.66 },
    { "id": "farley", "type": "interact", "text": "Snakk med Farley", "npc": "Innkeeper Farley" },
    { "id": "water", "type": "collect", "text": "Ha vann", "item_id": 159, "count": 1 }
  ],
  "objectives_ordered": false,
  "when": { "clock": "local", "start": "22:00", "end": "02:00" },
  "group": { "size": 4, "min_level": 17, "instance": "The Deadmines" },
  "reward": "",
  "min_level": 1,
  "party_quest": false,
  "active": true,
  "assigned_character_ids": []
}
```

Empty `assigned_character_ids` with `party_quest: true` offers the saga to everyone eligible by level.

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
- Admin (Smia MCP): optional `image` file on `POST`/`PATCH` `/v1/admin/quests`, or `PUT`/`DELETE` `/v1/admin/quests/{id}/image`

When present, `has_image` is true and `image_url` points at the GET path. The companion converts the cover to TGA and writes `Interface/AddOns/GutSaga/media/<quest_id>.tga` on sync (Classic Era cannot load loose PNGs as addon textures). The addon shows it under the description, above goals.

The player GET stays session-authenticated. Smia, which only has the admin token, fetches the same bytes at `GET /v1/admin/quests/{id}/image`. Admin quest list and admin quest get also include `image_signed_url` (absolute, expiring, no auth header) when `PUBLIC_BASE_URL` and `IMAGE_URL_SECRET` are set. That URL is `GET /v1/public/sagas/{id}/image` and returns only the image. See [API.md](API.md).

## After create / update

The companion syncs into `GutSagaData.lua`. Players need `/reload` in-game to see new or changed sagas.
