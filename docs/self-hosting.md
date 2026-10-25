# Self-hosting SagaBoard

Railway is one place you can run the API. It is not required. This repository includes Compose for Postgres and the API.

## Compose

```bash
docker compose up --build -d
```

The API listens on port 8080. Compose is for local development and a single host. Set real secrets outside the checked-in defaults. The known dev secrets are rejected unless `SAGABOARD_ALLOW_DEV_SECRETS=1`.

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | yes | Postgres connection string |
| `ADMIN_TOKEN` | for ops routes | Hard-delete a saga, or delete a group and its sagas. Cannot author sagas. |
| `BNET_CLIENT_ID` | for login | Battle.net OAuth client id |
| `BNET_CLIENT_SECRET` | for login | Stays on the API. Never put it in the companion. |
| `PUBLIC_BASE_URL` | for login and covers | Public origin, no trailing slash. Redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. |
| `IMAGE_URL_SECRET` | for signed covers | `openssl rand -hex 32` |
| `SAGABOARD_BOOTSTRAP_GROUP` | when old sagas have no group | `slug:Name`. A fresh database does nothing. |
| `SAGABOARD_OWNER_BATTLETAG` | no | Owner of that bootstrap group. Otherwise the oldest player who has a BattleTag. |
| `DISCORD_WEBHOOK_URL` | no | Copied once onto the bootstrap group when its webhook is empty. After that the group row is the source of truth. New groups stay quiet until the owner sets a webhook. |
| `SAGABOARD_DEV_TOKEN` | dev and CI only | Login bypass. It is not a stand-in for Battle.net. |

Each host registers its own Battle.net application and redirect URI. WoW Forever characters are not in Blizzard's profile API. The addon reports characters. A later check may mark them verified. Login does not wait for that.

## Companion

```bash
go run ./cmd/sagaboard --api-url https://sagaboard.example.com
```

Official release binaries bake the public URL with:

```bash
-X github.com/magnusnilsen/gutsaga/internal/version.DefaultAPIBaseURL=${SAGABOARD_API_URL}
```

`SAGABOARD_API_URL` is a GitHub Actions variable on the official repository. If it is empty, the companion says `--api-url` or `SAGABOARD_API_URL` is required.

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
