# Self-hosting Sagalore

Railway is one place you can run the API. It is not required. This repository includes Compose for Postgres and the API.

## Compose

```bash
docker compose up --build -d
```

The API listens on port 8080. Compose is for local development and a single host. Set real secrets outside the checked-in defaults. The known dev secrets are rejected unless `SAGALORE_ALLOW_DEV_SECRETS=1`. Each `SAGALORE_*` variable below still accepts the previous `OURSAGAS_*` name when the new one is unset.

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | yes | Postgres connection string |
| `ADMIN_TOKEN` | for ops routes | Hard-delete a saga, or delete a group and its sagas. Cannot author sagas. |
| `BNET_CLIENT_ID` | for login | Battle.net OAuth client id |
| `BNET_CLIENT_SECRET` | for login | Stays on the API. Never put it in the companion. |
| `PUBLIC_BASE_URL` | for login and covers | Public origin, no trailing slash. Redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. |
| `IMAGE_URL_SECRET` | for signed covers | `openssl rand -hex 32` |
| `SAGALORE_BOOTSTRAP_GROUP` | when old sagas have no group | `slug:Name`. A fresh database does nothing. |
| `SAGALORE_OWNER_BATTLETAG` | no | Owner of that bootstrap group. Otherwise the oldest player who has a BattleTag. |
| `DISCORD_WEBHOOK_URL` | no | Copied once onto the bootstrap group when its webhook is empty. After that the group row is the source of truth. New groups stay quiet until the owner sets a webhook. |
| `SAGALORE_DEV_TOKEN` | dev and CI only | Login bypass. It is not a stand-in for Battle.net. |
| `SAGALORE_MCP_SERVICE_SECRET` | for the public MCP | Shared secret Smia sends as `Authorization: Bearer`. Also send `X-Smia-Caller` (signed JWT) or `X-Sagalore-Caller-Subject` (Smia user id) and optional `X-Sagalore-Caller-Email`. `OURSAGAS_MCP_SERVICE_SECRET` is the fallback. Without the secret, those headers are rejected. |
| `SAGALORE_SMIA_ISSUER` | no | Allowed caller JWT `iss`. One origin, or several separated by commas. Default `https://smia.tools`. JWKS for each is `{issuer}/.well-known/smia-caller-jwks.json`. `OURSAGAS_SMIA_ISSUER` is the fallback. |
| `SAGALORE_REQUIRE_SMIA_JWT` | no | Set to `1` to reject `X-Sagalore-Caller-*` when `X-Smia-Caller` is absent. Default off. `OURSAGAS_REQUIRE_SMIA_JWT` is the fallback. |

## MCP

The Smia tools call this API as the signed-in Smia user. Prefer `X-Smia-Caller` (ES256 JWT, `aud` is this host's `PUBLIC_BASE_URL` origin). `X-Sagalore-Caller-Subject` still works until `SAGALORE_REQUIRE_SMIA_JWT=1`. That user links Battle.net once: `POST /v1/auth/link` (or the `link_url` on an `identity_not_linked` error) opens the existing Battle.net login, and the callback stores the link. After that, the same caller acts as the linked player. See [API.md](API.md).

Each host registers its own Battle.net application and redirect URI. WoW Forever characters are not in Blizzard's profile API. The addon reports characters. A later check may mark them verified. Login does not wait for that.

## Companion

```bash
go run ./cmd/sagalore --api-url https://sagalore.example.com
```

Official release binaries bake the public URL with:

```bash
-X github.com/magnusnilsen/gutsaga/internal/version.DefaultAPIBaseURL=${SAGALORE_API_URL}
```

`SAGALORE_API_URL` is a GitHub Actions variable on the official repository. The workflow also reads the previous variable `OURSAGAS_API_URL` when the new one is empty. If both are empty, the companion says `--api-url` or `SAGALORE_API_URL` is required.

Set `update.repo` to your fork, or disable updates, if you are not running the official builds. The module path stays `github.com/magnusnilsen/gutsaga`.

## Create a group and a saga

Use a `gst_` token from `POST /v1/me/tokens` (shown once):

```bash
curl -s -X POST "$API/v1/groups" \
  -H "Authorization: Bearer $GST" -H "Content-Type: application/json" \
  -d '{"slug":"tutorial","name":"Tutorial","locale":"en"}'

curl -s -X POST "$API/v1/sagas" \
  -H "Authorization: Bearer $GST" -H "Content-Type: application/json" \
  -d '{"title":"An evening in Goldshire","description":"Visit the inn.","party":true,"group":"tutorial","objectives":[{"id":"inn","type":"go","text":"Stand in Goldshire","zone":"Elwynn Forest"}]}'
```

If the player is in exactly one group, omit `group`.
