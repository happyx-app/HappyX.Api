# HappyX.Api

REST API for HappyX: members of a Discord server answer a daily mood survey (1-5, with an
optional message) and the server admin receives a weekly or monthly report.

Go, clean architecture, PostgreSQL. One API serves many Discord servers ("guilds") without
mixing their data.

## Layout

```
cmd/api                      entry point (config, wiring, graceful shutdown)
internal/domain              entities and rules (Guild, Member, MoodEntry, Report); no dependencies
internal/usecase             application logic + repository interfaces (ports)
internal/repository/postgres PostgreSQL adapters (pgx) and embedded migrations runner
internal/transport/http      JSON REST handlers and API key middleware
migrations                   SQL migrations, applied automatically on startup
```

Dependencies point inwards: `transport` and `repository` depend on `usecase`, which depends on `domain`.

## Multi-server isolation

The Discord guild ID is the tenant key. Every route that touches tenant data is under
`/v1/guilds/{guildID}`, and every query on members and moods filters by that guild. The same
Discord user in two servers is two separate members with separate histories. See
`TestGuildIsolation`.

## Anonymity

Guilds are anonymous by default: reports include messages but never who wrote them. An admin can
turn this off with `PATCH /v1/guilds/{guildID}` `{"anonymous": false}`.

## Credentials

All secrets come from environment variables; nothing is read from files in the repo.

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | PostgreSQL connection string |
| `API_KEY_HASHES` | Comma separated SHA-256 hex digests of the API keys allowed to call `/v1` |
| `HTTP_ADDR` | Listen address, default `:8080` |
| `DISCORD_CLIENT_ID` | Discord application ID. Set it to enable the web dashboard login; the next three are then required |
| `DISCORD_CLIENT_SECRET` | Discord application OAuth2 secret |
| `WEB_URL` | Public URL of the dashboard, e.g. `https://happyx.example.com` |
| `SESSION_SECRET` | At least 32 random characters to sign dashboard sessions (`openssl rand -hex 32`) |

Clients (for now, only the Discord bot) send `Authorization: Bearer <key>`. The API stores only
the hash, so a leaked config does not leak a usable key. Give each client its own key so it can
be rotated on its own:

```sh
KEY=$(openssl rand -hex 32); echo "key: $KEY"; echo -n "$KEY" | sha256sum
```

Put the key in the bot's environment and the hash in `API_KEY_HASHES`. Locally use a git-ignored
`.env` (see `.env.example`); in production use the host's secret store.

## Web dashboard

People log in to [HappyX.Web](https://github.com/moixblau/HappyX.Web) with Discord OAuth2 (scopes
`identify` and `guilds`). The dashboard's nginx forwards `/api/web/...` to this API's `/web/...`,
so the API itself doesn't need to be public. In the Discord application (OAuth2 > Redirects) add
`<WEB_URL>/api/web/auth/callback`.

A user sees a server if they are the admin the bot sends reports to, or the owner or an
Administrator of the server on Discord. Discord permissions are read at login; the session is an
HMAC-signed HttpOnly cookie that lasts 12 hours and the API keeps no session state. The Discord
access token is used once at login and never stored. Reports keep the server's anonymity
setting. Servers a user can't see answer 404, like servers without HappyX.

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/web/auth/login` | redirects to Discord |
| GET | `/web/auth/callback` | Discord redirects here; sets the session and goes back to `WEB_URL` |
| POST | `/web/auth/logout` | 204 |
| GET | `/web/me` | `{user, guilds}`: the logged-in user and the servers they can see |
| GET | `/web/guilds/{guildID}/report` | same query and body as the `/v1` report |

`/web` routes use the session cookie and `/v1` routes use API keys; neither works on the other.

## API

| Method | Path | Body | Notes |
| --- | --- | --- | --- |
| GET | `/health` | | public |
| GET | `/v1/guilds` | | active guilds (for the bot's scheduler) |
| POST | `/v1/guilds` | `discord_guild_id`, `name`, `admin_discord_user_id` | register on join; idempotent, keeps settings |
| GET | `/v1/guilds/{guildID}` | | |
| PATCH | `/v1/guilds/{guildID}` | any of `anonymous`, `timezone`, `survey_hour`, `report_frequency` (`weekly`/`monthly`), `language` (`en`, `es`, `pt-BR`…; default `en`), `admin_discord_user_id` | |
| DELETE | `/v1/guilds/{guildID}` | | bot removed: stops surveys, keeps data |
| GET | `/v1/guilds/{guildID}/members` | | active members |
| PUT | `/v1/guilds/{guildID}/members/{userID}` | `username`, `active` (optional) | create or rename; `active` omitted keeps an existing member's opt-in state, `true` opts in |
| DELETE | `/v1/guilds/{guildID}/members/{userID}` | | opt out |
| PUT | `/v1/guilds/{guildID}/members/{userID}/mood` | `mood` (1-5), `message` (optional, ≤500), `day` (optional `YYYY-MM-DD`) | one answer per day; answering again replaces it |
| GET | `/v1/guilds/{guildID}/report` | `?from=YYYY-MM-DD&to=YYYY-MM-DD` | without a range: last complete week or month |
| POST | `/v1/guilds/{guildID}/dispatches` | `kind` (`survey`/`report`), `period` (`YYYY-MM-DD`) | 201: go ahead and send; 409: already sent. Makes scheduled sends exactly-once across restarts |

Errors are `{"error": "..."}` with 400 (validation), 401, 403 (not a member), 404, 409 (inactive guild, or dispatch already claimed).

## Development

```sh
docker run -d --name happyx-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=happyx_test -p 55432:5432 postgres:18-alpine
export TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/happyx_test?sslmode=disable'
go test -p 1 ./...
```

Database tests are skipped when `TEST_DATABASE_URL` is not set. To run the whole stack, use the
`docker-compose.yml` in [HappyX](https://github.com/moixblau/HappyX).
