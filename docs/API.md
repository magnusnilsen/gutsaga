# SagaBoard HTTP API

Players sign in with Battle.net. The companion session is `gss_` and lasts 30 days. External clients use a personal token `gst_`. `ADMIN_TOKEN` is ops only.

The module and many env vars still say `GUTSAGA_`. Paths stay `/v1/sagas`.

## Login

`GET /v1/auth/bnet/start?attempt=` redirects to `https://oauth.battle.net/authorize` (authorization code, PKCE, scope `openid wow.profile`).

`GET /v1/auth/bnet/callback` exchanges the code with `BNET_CLIENT_ID` and `BNET_CLIENT_SECRET`, reads userinfo (`id`, `battletag`), merges or creates the player, and stores the session on the attempt.

`GET /v1/auth/bnet/poll?attempt=` returns the session when the browser flow finishes, or `202` while it is pending.

The redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. Each host registers its own Battle.net app.

`POST /v1/auth/dev` with `Authorization: Bearer $GUTSAGA_DEV_TOKEN` issues a session without calling Blizzard. Dev and CI only.

Login merges an existing player by `bnet_account_id`, else by BattleTag. No other key.

## Tokens

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/me/tokens` | Mint `gst_`. The raw token is in the response once. |
| `DELETE` | `/v1/me/tokens/{id}` | Revoke. |

Saga and group writes reject `ADMIN_TOKEN`, `created_by`, and user-identity headers. The signed-in account is the author.

## Groups

Session or `gst_`:

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/groups` | Create. Caller becomes owner. |
| `GET` | `/v1/groups` | Groups the account belongs to. |
| `POST` | `/v1/groups/{id}/invites` | Owner. Body `{name, realm, battletag?}`. |
| `POST` | `/v1/invites/{id}/accept` | Join the account. Marks that character participating. |
| `POST` | `/v1/invites/{id}/decline` | Permanent for that invite. |
| `POST` | `/v1/groups/{id}/leave` | Leave. If the owner leaves, the oldest remaining member becomes owner. |
| `DELETE` | `/v1/groups/{id}/members/{playerID}` | Owner removes a member. |
| `PUT` | `/v1/groups/{id}/discord` | Owner. `{"webhook_url":"..."}`. |
| `PUT` | `/v1/groups/{id}/locale` | Owner. `en` or `nb`. |
| `PATCH` | `/v1/characters/{id}` | `{"participating":true}`. |

## Sagas

`GET /v1/sync` returns sagas in groups the player belongs to, that player's progress, and completions limited to those groups. Party credit for a kill counts only characters whose accounts are members of the same group.

`POST /v1/sagas` — any member. Body field `group` is a slug when it is a string. If the player is in exactly one group it may be omitted. Non-members get `403`.

`PATCH /v1/sagas/{id}` — the author. `changes` is required when the text changes. A cover-only upload does not bump the version.

`POST /v1/complete` — the character's account must be a member of the saga's group.

## Ops

| Method | Path | |
| --- | --- | --- |
| `DELETE` | `/v1/admin/sagas/{id}` | Hard-delete one saga. |
| `DELETE` | `/v1/admin/groups/{id}` | Delete the group and its sagas. |

Seed runs from `gutsaga-api seed` or `make dev`. It does not run on server boot.

## Errors

`error` is the code and the contract. `message` is English. Clients may translate known codes. Discord text uses the group's locale, default English.

See [sagas-api.md](sagas-api.md) for the saga document and [self-hosting.md](self-hosting.md) for deployment.
