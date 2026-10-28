# Sagalore HTTP API

Players sign in with Battle.net. The companion session is `gss_` and lasts 30 days. External clients use a personal token `gst_`. `ADMIN_TOKEN` is ops only.

The Go module path is still `github.com/magnusnilsen/gutsaga` until the public snapshot. Paths stay `/v1/sagas`.

## Login

`GET /v1/auth/bnet/start?attempt=` redirects to `https://oauth.battle.net/authorize` (authorization code, PKCE, scope `openid wow.profile`).

`GET /v1/auth/bnet/callback` exchanges the code with `BNET_CLIENT_ID` and `BNET_CLIENT_SECRET`, reads userinfo (`id`, `battletag`), merges or creates the player, and stores the session on the attempt.

`GET /v1/auth/bnet/poll?attempt=` returns the session when the browser flow finishes, or `202` while it is pending.

The redirect URI is `{PUBLIC_BASE_URL}/v1/auth/bnet/callback`. Each host registers its own Battle.net app.

`POST /v1/auth/dev` with `Authorization: Bearer $SAGALORE_DEV_TOKEN` issues a session without calling Blizzard. Dev and CI only.

Login merges an existing player by `bnet_account_id`, else by BattleTag. No other key.

## MCP identity

Smia calls the API with a service credential and the caller's own identity. Set `SAGALORE_MCP_SERVICE_SECRET` (`OURSAGAS_MCP_SERVICE_SECRET` is the fallback).

| Header | |
| --- | --- |
| `Authorization` | `Bearer $SAGALORE_MCP_SERVICE_SECRET` |
| `X-Smia-Caller` | Smia-signed ES256 JWT. When present it must verify, and it wins over the caller headers below. |
| `X-Sagalore-Caller-Subject` | Smia user id. Used when `X-Smia-Caller` is absent. |
| `X-Sagalore-Caller-Email` | Optional. Stored on the link. Not a lookup key. |

The headers are not an identity by themselves. A request that sends them, or `X-Smia-Caller`, without the service secret is `401`, including when it also sends a `gst_` or `gss_` token. The API does not fall through to that token. A JWT that fails verification does not fall back to `X-Sagalore-Caller-*`.

`X-Smia-Caller` is a JWT signed with ES256 (P-256), lifetime at most 5 minutes. `kid` selects a key from `{SAGALORE_SMIA_ISSUER}/.well-known/smia-caller-jwks.json` (default issuer `https://app.smia.io`). The API checks the signature, `iss` (the Smia origin), `aud` (the `PUBLIC_BASE_URL` origin, scheme and host), `exp` (30 seconds of clock skew), and `jti` (rejected if seen again before `exp`). `sub` is the Smia user id. `email` is stored on the link. `hub`, `listing_id`, and `tool` must be present. Keys are cached and refreshed when `kid` is unknown.

`SAGALORE_REQUIRE_SMIA_JWT=1` rejects header-only identity. The default is off, so `X-Sagalore-Caller-Subject` still works until Smia sends the JWT. `OURSAGAS_SMIA_ISSUER` and `OURSAGAS_REQUIRE_SMIA_JWT` are the fallbacks.

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/auth/link` | Service credential plus caller headers. Returns `{attempt, url, poll_url}`. `url` is `https://oauth.battle.net/authorize` from the same PKCE flow as `/v1/auth/bnet/start`. |
| `GET` | `/v1/auth/bnet/callback` | Existing callback. When the attempt was a link, it binds provider `smia` and the subject to the Battle.net player. |
| `GET` | `/v1/auth/bnet/poll` | Existing poll for that attempt. |

An authenticated caller with no link gets `403`:

```json
{"error":"identity_not_linked","message":"...","link_url":"https://oauth.battle.net/authorize?...","attempt":"..."}
```

`link_url` is the same authorize URL. After the player finishes Battle.net login, the same caller acts as that player.

The service-secret comparison runs before `CallerVerifier.Verify`. `MCPCallerVerifier` checks `X-Smia-Caller` when it is set, and otherwise reads the caller headers. Neither path skips the service secret.

## Tokens

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/me/tokens` | Mint `gst_`. The raw token is in the response once. |
| `DELETE` | `/v1/me/tokens/{id}` | Revoke. |

Saga and group writes reject `ADMIN_TOKEN`, `created_by`, and user-identity headers. The signed-in account is the author. An MCP call is that linked player, and only when the service secret matches.

## Groups

Session, `gst_`, or a linked MCP caller:

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/groups` | Create. Caller becomes owner. |
| `GET` | `/v1/groups` | Groups the account belongs to. |
| `GET` | `/v1/groups/{id}/members` | Members of a group you belong to. |
| `POST` | `/v1/groups/{id}/invites` | Owner. Body `{name, realm, battletag?}`. |
| `GET` | `/v1/groups/{id}/invites?status=pending` | Owner or member. Pending outgoing invites: character `name`, `realm`, `invited_at`. Other `status` values are `400`. |
| `DELETE` | `/v1/groups/{id}/invites/{inviteId}` | Owner revokes a pending invite. A member or outsider gets `403`. An accepted or declined invite is `409`. |
| `GET` | `/v1/me/invites` | Pending invites addressed to the caller (character or BattleTag). This does not create include-invites. `GET /v1/sync` still does, for alts that are not participating. |
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
