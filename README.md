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

Clients (for now, only the Discord bot) send `Authorization: Bearer <key>`. The API stores only
the hash, so a leaked config does not leak a usable key. Give each client its own key so it can
be rotated on its own:

```sh
KEY=$(openssl rand -hex 32); echo "key: $KEY"; echo -n "$KEY" | sha256sum
```

Put the key in the bot's environment and the hash in `API_KEY_HASHES`. Locally use a git-ignored
`.env` (see `.env.example`); in production use the host's secret store.

When the web app arrives, users will log in with Discord OAuth2 and only see guilds they administer.

## API

| Method | Path | Body | Notes |
| --- | --- | --- | --- |
| GET | `/health` | | public |
| GET | `/v1/guilds` | | active guilds (for the bot's scheduler) |
| POST | `/v1/guilds` | `discord_guild_id`, `name`, `admin_discord_user_id` | register on join; idempotent, keeps settings |
| GET | `/v1/guilds/{guildID}` | | |
| PATCH | `/v1/guilds/{guildID}` | any of `anonymous`, `timezone`, `survey_hour`, `report_frequency` (`weekly`/`monthly`), `admin_discord_user_id` | |
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
docker run -d --name happyx-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=happyx_test -p 55432:5432 postgres:17-alpine
export TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/happyx_test?sslmode=disable'
go test -p 1 ./...
```

Database tests are skipped when `TEST_DATABASE_URL` is not set. To run the whole stack, use the
`docker-compose.yml` in [HappyX](https://github.com/moixblau/HappyX).
