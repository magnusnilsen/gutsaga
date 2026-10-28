# Realtime

The companion used to learn about sagas by calling `GET /v1/sync` every ten minutes. It still does that as a fallback. The fast path is a WebSocket.

Discord and addon chrome follow the group or companion language. This note is English.

## Socket

`GET /v1/ws`, `Authorization: Bearer` with the same Sagalore session (`gss_…`) or personal token (`gst_…`) as `/v1/sync`. A bad or missing token is `401` and the connection is not upgraded. The token is a header, not a query parameter, so it does not land in access logs.

The companion is not a browser. The server does not check `Origin`. The bearer is the credential.

Each text frame is one JSON object:

| `type` | Who receives it | When |
|---|---|---|
| `saga.created` | members of the saga's group | a saga row is inserted (`version` is 1) |
| `saga.updated` | members of the saga's group | `quests.version` changes and the saga stays or becomes active |
| `saga.retired` | members of the saga's group | `quests.version` changes and `active` goes from true to false |
| `saga.completed` | members of the saga's group | a new progress row with `status = completed` |
| `level.changed` | only that `player_id` | `characters.level` actually changes |
| `ping` | the socket that is being kept warm | every 25s, from the process that holds the socket |

```json
{"type":"saga.created","saga_id":"cf031479-7288-4508-b139-f23f1e322026","version":1}
{"type":"saga.updated","saga_id":"cf031479-7288-4508-b139-f23f1e322026","version":2}
{"type":"saga.retired","saga_id":"cf031479-7288-4508-b139-f23f1e322026","version":3}
{"type":"saga.completed","saga_id":"cf031479-7288-4508-b139-f23f1e322026","version":1}
{"type":"level.changed","player_id":"…"}
{"type":"ping"}
```

`version` on a completion is the quest version the player had loaded, not necessarily the current draft. `level.changed` has no `saga_id`.

The frame is a hint. The companion calls `GET /v1/sync` and rewrites `SagaloreData.lua`. It does not apply the event as the saga text. Empty fields are omitted. The payload stays far under Postgres `NOTIFY`'s 8000-byte cap.

A second completion of the same character and saga does not insert a row and does not emit `saga.completed`. Setting the same level again does not emit `level.changed`. Updating only `image_bytes` does not mention `version` or `active`, so a cover image waits for the next sync (the ten-minute poll, a later saga event, or a reconnect).

## Several API processes

Migration `007_realtime_notify.sql` installs triggers. On commit they `pg_notify('gutsaga_events', …)`. Each API process holds **one dedicated connection** (`application_name = gutsaga-realtime`) and `LISTEN`s. A pooled connection would be reused and drop the subscription.

Railway can run more than one replica. No sticky sessions: the replica that committed the saga and the replica that holds the player's socket can be different. Both hear the notify. Each replica fans the event out only to the sockets it accepted.

A new write path in `api/` does not need to call Go code. Bumping `quests.version` (what `UpdateQuest` already does) is enough. A path that writes some other table can emit the same JSON:

```sql
SELECT pg_notify(
  'gutsaga_events',
  '{"type":"saga.updated","saga_id":"<uuid>","version":2}'
);
```

## Companion

`http://` API URLs become `ws://`, `https://` becomes `wss://`, path `/v1/ws`.

On each event except `ping`, and on every reconnect, the companion syncs and rewrites `SagaloreData.lua`. Bursts within 200ms become one sync. The ten-minute poll stays.

If the visible saga list changed, the companion logs a line (`Ny saga: …`, `Saga oppdatert: …`, and so on). It does **not** raise a desktop banner — Discord carries the player-facing alert.

The first sync after the program starts stays quiet in that sense. Startup already asks for `/reload`.

WoW reads `SagaloreData.lua` only at login and `/reload`. The program cannot press keys in the client. The file also gets `notice` and `notice_id`. After `/reload` the addon prints that notice once in chat and remembers it in `SagaloreDB.seen_notice`.

Reconnect waits 1s, 2s, 4s, … capped at 60s, with ±20% jitter. A `401` clears the session and starts Battle.net login again (or the dev-token bypass). If no data frame arrives for 70 seconds the companion treats the socket as dead. The server's text `ping` is that data frame.

## Railway

Source: [Public networking specs and limits](https://docs.railway.com/networking/public-networking/specs-and-limits) and [WebSockets vs SSE](https://docs.railway.com/guides/sse-vs-websockets).

- The edge accepts the HTTP/1.1 upgrade. Nothing extra is configured on the service. `railway.json` stays a normal HTTP service with health check `GET /healthz`.
- TLS ends at the proxy. Players connect with `wss://` because the public API URL is `https://`. `X-Forwarded-Proto` is `https`.
- WebSocket connections are **exempt** from the HTTP duration and idle limits and may stay open with no traffic. Ordinary HTTP/1.1 connections that never upgrade are closed after 60 seconds of idleness. HTTP requests otherwise run at most 15 minutes when bytes are flowing, and 5 minutes when they are not. Those caps do not apply once the upgrade has succeeded.
- The server still sends a WebSocket ping and a text `{"type":"ping"}` every 25 seconds. NATs, sleeping laptops, and a connection that only looked like a WebSocket still go quiet. 25 seconds is under the 60-second HTTP idle limit. The Go `http.Server` does **not** set `ReadTimeout` or `WriteTimeout`; those would cut the socket. `ReadHeaderTimeout` stays 10 seconds and only covers the upgrade headers.
- Header budget is 32 KB. The bearer fits.
- Inbound public traffic is TLS. Do not point the companion at plain `http://` on the Railway hostname.
- A deploy sends `SIGTERM`. The process closes sockets with close code `1001` (Going Away), stops the `LISTEN` connection, then shuts the HTTP server down within 10 seconds. The companion reconnects and syncs, so a saga committed during the rollout is not lost.
- Postgres `NOTIFY` is what makes a second replica correct. One replica's memory is not enough.

## Tests

```bash
export DATABASE_URL=postgres://gutsaga:gutsaga@127.0.0.1:5432/gutsaga_test?sslmode=disable
go test ./internal/realtime ./internal/syncdata ./internal/notify ./internal/app ./api
```

`TestRealtimeTwoInstances` runs two listeners against one database, which is the two-replica case.
