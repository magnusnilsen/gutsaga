# SagaBoard

SagaBoard is a saga log for a World of Warcraft group. One person plays. The desktop companion installs the addon, signs in with Battle.net, and syncs sagas. The addon has no HTTP of its own: it writes chat lines and SavedVariables, and the companion posts them.

API paths stay `/v1/sagas`. A saga is still a saga. The JSON field `group` on a kill step or a saga is the party-size rule. Who plays together is a row in `groups`.

## Try it locally

You need Docker, Go, and a WoW client. Postgres and the API come from Compose. Login in development uses `SAGABOARD_DEV_TOKEN` so you do not need a Battle.net app for the first run.

```bash
make dev
```

That starts Postgres and the API, waits for `/healthz`, and seeds the Elwynn tutorial sagas. In another terminal:

```bash
SAGABOARD_DEV_TOKEN=dev-token SAGABOARD_ADDON_SRC=addon/SagaBoard \
  go run ./cmd/sagaboard --api-url http://localhost:8080 --client classic_era
```

The API container must have the same `SAGABOARD_DEV_TOKEN`. `make dev` sets `dev-token` for local use only.

The companion installs `Interface/AddOns/SagaBoard`. In game, `/reload`, then `/sagaboard` or `/sb`.

If the database already has sagas and no group, set `SAGABOARD_BOOTSTRAP_GROUP=slug:Name` before the API starts. See [docs/self-hosting.md](docs/self-hosting.md).

## What you set before a real login

- Register a Battle.net application for this host. The redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. Blizzard will not accept a random loopback port, so the secret stays on the API.
- `BNET_CLIENT_ID` and `BNET_CLIENT_SECRET` on the API.
- `PUBLIC_BASE_URL` for that redirect and for signed cover URLs.
- Official release binaries get the public API URL from the GitHub Actions variable `SAGABOARD_API_URL` (`-ldflags`). A self-hosted companion passes `--api-url`.

## Groups

A player is a Battle.net account (`bnet_account_id` and BattleTag). Characters come from the addon. A name and realm sticks to the first account that linked it. If the character line includes a BattleTag, it must match the logged-in account.

Membership is the account. Any member may create sagas in the group. The owner invites, removes members, and sets Discord and locale. If you are in exactly one group, the saga body may omit `group`. Otherwise `group` is the group slug (a string). The party-size object is still `group` when it is an object; it may include `slug` when you need both.

Invites are addressed to a character. Accept joins that Battle.net account and marks the character participating. Decline sticks. Not now is a 12-hour snooze in SavedVariables.

## Personal tokens

Players who only play never need a token. A long-lived `gst_` token is for external HTTP clients. `POST /v1/me/tokens` returns the raw token once. `DELETE /v1/me/tokens/{id}` revokes it. The tray can mint and revoke one.

`ADMIN_TOKEN` is for operators: hard-delete a saga, or delete a group and its sagas. It cannot author sagas.

## Docs

- [docs/self-hosting.md](docs/self-hosting.md) — Compose, env, Battle.net app, curl
- [docs/API.md](docs/API.md) — HTTP
- [docs/sagas-api.md](docs/sagas-api.md) — saga JSON
- [docs/character-opt-in.md](docs/character-opt-in.md) — participating characters
- [docs/openapi.yaml](docs/openapi.yaml)
- [SECURITY.md](SECURITY.md) and [CONTRIBUTING.md](CONTRIBUTING.md)

## Updates

Release assets are `sagaboard-darwin-arm64`, `sagaboard-darwin-amd64`, `sagaboard-windows-amd64.exe`, and `SagaBoard-addon.zip`. The default update repo is `magnusnilsen/gutsaga`. Self-hosters set `update.repo` or turn updates off. Binaries are not signed.
