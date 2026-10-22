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

When present, `has_image` is true and `image_url` points at the GET path. The companion converts the cover to TGA and writes `Interface/AddOns/GutSaga/media/<quest_id>.tga` on sync (Classic Era cannot load loose PNGs as addon textures). The addon shows it under the description, above goals.

## After create / update

The companion syncs into `GutSagaData.lua`. Players need `/reload` in-game to see new or changed sagas.
