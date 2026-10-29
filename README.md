# Sagalore

Sagalore is a saga log for a World of Warcraft group. One person plays. The desktop companion installs the addon, signs in with Battle.net, and syncs sagas. The addon has no HTTP of its own: it writes chat lines and SavedVariables, and the companion posts them.

API paths stay `/v1/sagas`. A saga is still a saga. The JSON field `group` on a kill step or a saga is the party-size rule. Who plays together is a row in `groups`.

The product was renamed from OurSagas to Sagalore. The repository and Go module stay `gutsaga`. See [CHANGELOG.md](CHANGELOG.md).

## Rename from OurSagas

Existing WoW Classic characters keep their progress:

- The companion copies `WTF/.../SavedVariables/OurSagas.lua` to `Sagalore.lua` and renames the `OurSagasDB` global. A `Sagalore.lua` that already has progress is left as-is. On the first addon load, if `OurSagasDB` is still in memory and `SagaloreDB` has no progress, the addon copies it.
- The companion copies `Interface/AddOns/OurSagas/OurSagasData.lua` and `media/*.tga` into `Interface/AddOns/Sagalore`, then removes the old addon folder (or renames it to `OurSagas.disabled` if the folder is locked) so the client does not load two addons.
- Settings move from the OurSagas config folder to Sagalore when the new folder is empty. Older GutSaga and SagaBoard folders are still picked up.
- `/sagalore` and `/sl` open the log.
- Chat lines use `[Sagalore]`. The companion still reads `[OurSagas]` lines already in the chat log. Addon messages sent with the old prefix are still accepted.
- `SAGALORE_*` environment variables replace `OURSAGAS_*`. The old names are still read when the new one is unset. Official builds read the GitHub Actions variable `SAGALORE_API_URL`, and fall back to `OURSAGAS_API_URL`.
- Release files are `sagalore-*` and `Sagalore-addon.zip`. The same bytes are also uploaded as `oursagas-*` and `OurSagas-addon.zip` so an already-installed companion can update once.

## Try it locally

You need Docker, Go, and a WoW client. Postgres and the API come from Compose. Login in development uses `SAGALORE_DEV_TOKEN` so you do not need a Battle.net app for the first run.

```bash
make dev
```

That starts Postgres and the API, waits for `/healthz`, and seeds the Elwynn tutorial sagas. In another terminal:

```bash
SAGALORE_DEV_TOKEN=dev-token SAGALORE_ADDON_SRC=addon/Sagalore \
  go run ./cmd/sagalore --api-url http://localhost:8080 --client classic_era
```

The API container must have the same `SAGALORE_DEV_TOKEN`. `make dev` sets `dev-token` for local use only.

The companion installs `Interface/AddOns/Sagalore`. In game, `/reload`, then `/sagalore` or `/sl`.

If the database already has sagas and no group, set `SAGALORE_BOOTSTRAP_GROUP=slug:Name` before the API starts. See [docs/self-hosting.md](docs/self-hosting.md).

## What you set before a real login

- Register a Battle.net application for this host. The redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. Blizzard will not accept a random loopback port, so the secret stays on the API.
- `BNET_CLIENT_ID` and `BNET_CLIENT_SECRET` on the API.
- `PUBLIC_BASE_URL` for that redirect and for signed cover URLs.
- Official release binaries get the public API URL from the GitHub Actions variable `SAGALORE_API_URL` (`-ldflags`). A self-hosted companion passes `--api-url`.

## Groups

A player is a Battle.net account (`bnet_account_id` and BattleTag). Characters come from the addon. A name and realm sticks to the first account that linked it. If the character line includes a BattleTag, it must match the logged-in account.

Membership is the account. Any member may create sagas in the group. The owner invites, removes members, and sets Discord and locale. If you are in exactly one group, the saga body may omit `group`. Otherwise `group` is the group slug (a string). The party-size object is still `group` when it is an object; it may include `slug` when you need both.

Invites are addressed to a character. Accept joins that Battle.net account and marks the character participating. Decline sticks. Not now is a 12-hour snooze in SavedVariables.

## Leaderboards

A group leaderboard is how many sagas each character has completed. It is not points, and it only includes characters whose Battle.net account is in that group. `GET /v1/groups/{id}/leaderboard?period=all|month|week|today` ranks them. Periods use the Europe/Oslo calendar: today, the Monday-starting week, and the calendar month.

The addon records `started_at` in SavedVariables when the first goal of a saga gains progress (the first rabbit of "drop 5 rabbits", not the fifth). The companion sends that time with the completion. The server stores `duration_sec`. Saga activity can be sorted by that duration or by newest completion. The addon shows both the leaderboard and the activity sort from `SagaloreData.lua`, which the companion writes from `GET /v1/sync`.

Finish times are whatever the addon reported. They are not verified. Repeatable sagas are not implemented. The progress table can already store more than one finish per character and saga, and the API still keeps the first one.

## Personal tokens

Players who only play never need a token. A long-lived `gst_` token is for external HTTP clients. `POST /v1/me/tokens` returns the raw token once. `DELETE /v1/me/tokens/{id}` revokes it. The tray can mint and revoke one.

Other services can register as OAuth clients and send the player through Battle.net and a consent page. The access token is also a `gst_` token, so the same API checks accept it. See [docs/oauth.md](docs/oauth.md).

`ADMIN_TOKEN` is for operators: hard-delete a saga, or delete a group and its sagas. It cannot author sagas.

## Docs

- [docs/self-hosting.md](docs/self-hosting.md) — Compose, env, Battle.net app, curl
- [docs/API.md](docs/API.md) — HTTP
- [docs/oauth.md](docs/oauth.md) — OAuth 2.1 for external clients
- [docs/sagas-api.md](docs/sagas-api.md) — saga JSON
- [docs/character-opt-in.md](docs/character-opt-in.md) — participating characters
- [docs/openapi.yaml](docs/openapi.yaml)
- [SECURITY.md](SECURITY.md) and [CONTRIBUTING.md](CONTRIBUTING.md)

## Updates

Release assets are `sagalore-darwin-arm64`, `sagalore-darwin-amd64`, `sagalore-windows-amd64.exe`, and `Sagalore-addon.zip`. The default update repo is `magnusnilsen/gutsaga`. Self-hosters set `update.repo` or turn updates off. Binaries are not signed.
