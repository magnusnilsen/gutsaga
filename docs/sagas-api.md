# Saga documents

`POST /v1/sagas` and `PATCH /v1/sagas/{id}` take JSON. The signed-in player is the author. Do not send `created_by`.

```json
{
  "title": "An evening in Goldshire",
  "description": "Visit Lion's Pride Inn.",
  "party": true,
  "group": "tutorial",
  "objectives": [
    {"id": "inn", "type": "go", "text": "Stand in Goldshire", "zone": "Elwynn Forest"}
  ]
}
```

`group` as a string is the tenant slug. Omit it when the player is in exactly one group.

The party-size rule is a different field with the same name when the value is an object. Do not rename it. On a kill step it is `group` inside the objective (`level`, `size`, `instance`). On a saga it is the roster checked at completion (`size`, `min_level`, `instance`). To send both a slug and a party rule, put `slug` inside the object:

```json
"group": {"slug": "tutorial", "size": 4, "min_level": 17, "instance": "The Deadmines"}
```

Saga titles and objective text are author content. They are not translated. Chrome around them uses the English or Norwegian catalog.

A `talk` step (older name `interact`) completes when the player opens that NPC's dialogue or uses the object. Targeting the NPC is not enough. If the NPC has no gossip window, the step also completes when the player right-clicks them, or targets them and uses `/talk`. The tracker does not append the player's current zone or subzone to a goal line.

A new saga starts at version 1. Text edits require `changes` and bump the version. Completions keep the version the character had loaded.

`GET /v1/sync` is what the companion writes into `SagaloreData.lua`. The addon reads it on `/reload`.

Covers: `PUT /v1/sagas/{id}/image`. The companion converts the PNG to TGA under `Interface/AddOns/Sagalore/media/`. Signed public URLs use `PUBLIC_BASE_URL` and `IMAGE_URL_SECRET`.
