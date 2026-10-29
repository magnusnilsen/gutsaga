# Changelog

## Leaderboards

Group members can read `GET /v1/groups/{id}/leaderboard?period=all|month|week|today`. The rank is how many distinct sagas a character has completed in that Oslo calendar window. There are no points. `GET /v1/sync` includes the same boards so the addon can show them. Any member of the saga's group can read `GET /v1/sagas/{id}`. Saga activity accepts `sort=completion_time` (fastest first) or `sort=completed_at` (newest first) and includes `duration_sec`. The addon records `started_at` when the first goal gains progress. Repeatable sagas are not enabled; the progress table can already store more than one finish per character and saga.

## OAuth 2.1 authorization server

External clients can register, send the player through Battle.net and a consent page, and call the API with a `gst_` access token. Refresh tokens rotate. Reusing an old refresh token revokes that client. The consent cookie is a short-lived sign-in for that page, not an API session. See [docs/oauth.md](docs/oauth.md).

## Group invites

Groups gained `GET /v1/groups/{id}/invites?status=pending` (owner or member), `DELETE /v1/groups/{id}/invites/{inviteId}` (owner), and `GET /v1/me/invites`.

## Rename: OurSagas is now Sagalore

The addon, companion, and docs use the name Sagalore. The git repository stays `gutsaga`, and the Go module stays `github.com/magnusnilsen/gutsaga`.

What moves for someone who already plays:

- SavedVariables. The companion copies `SavedVariables/OurSagas.lua` to `SavedVariables/Sagalore.lua` and renames `OurSagasDB` to `SagaloreDB`. WoW only loads the file that matches the addon folder, so this copy is what keeps Classic progress. If `Sagalore.lua` already has completions, objectives, pending resets, invites, or a seen notice, it is not overwritten. On first load the addon also copies an in-memory `OurSagasDB` when `SagaloreDB` has no progress (both addons loaded in one session, or a file that still uses the old global).
- Addon folder. The companion installs `Interface/AddOns/Sagalore`, copies `OurSagasData.lua` to `SagaloreData.lua` and `media/*.tga` covers, then removes `Interface/AddOns/OurSagas`. If that folder is locked it is renamed to `OurSagas.disabled`, which the client will not load. That avoids two addons in the addon list.
- Companion settings move from the OurSagas config directory to Sagalore when the new directory is empty.
- Slash commands. `/sagalore` and `/sl` open the log. `/oursagas` and `/os` are not registered.
- Chat and addon messages. New lines use `[Sagalore]` and the `Sagalore` addon-message prefix. The companion still parses `[OurSagas]` chat lines, and the addon still accepts the old addon-message prefix.
- Environment variables. `SAGALORE_*` is the name to set. `OURSAGAS_*` is still read when the new variable is empty, including `SAGALORE_API_URL`, `SAGALORE_DEV_TOKEN`, `SAGALORE_BOOTSTRAP_GROUP`, and the rest.
- Release assets are `sagalore-*` and `Sagalore-addon.zip`. The release also uploads those files under the previous `oursagas-*` and `OurSagas-addon.zip` names so a companion that has not updated yet can find them.

Outside this repository, still named OurSagas until someone changes them:

- The GitHub Actions repository variable `OURSAGAS_API_URL` (the workflow falls back to it; set `SAGALORE_API_URL` when you can).
- Railway (or any host) environment variables that still use the `OURSAGAS_` prefix, if they were set in the dashboard: `OURSAGAS_DEV_TOKEN`, `OURSAGAS_BOOTSTRAP_GROUP`, `OURSAGAS_OWNER_BATTLETAG`, `OURSAGAS_ALLOW_DEV_SECRETS`.
