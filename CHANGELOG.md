# Changelog

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
