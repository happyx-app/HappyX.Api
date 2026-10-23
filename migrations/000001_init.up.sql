CREATE TABLE guilds (
    id                    BIGSERIAL PRIMARY KEY,
    discord_id            TEXT NOT NULL UNIQUE,
    name                  TEXT NOT NULL,
    admin_discord_id      TEXT NOT NULL,
    anonymous             BOOLEAN NOT NULL DEFAULT TRUE,
    timezone              TEXT NOT NULL DEFAULT 'Europe/Madrid',
    survey_hour           SMALLINT NOT NULL DEFAULT 13 CHECK (survey_hour BETWEEN 0 AND 23),
    report_frequency      TEXT NOT NULL DEFAULT 'weekly' CHECK (report_frequency IN ('weekly', 'monthly')),
    active                BOOLEAN NOT NULL DEFAULT TRUE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE members (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    discord_id  TEXT NOT NULL,
    username    TEXT NOT NULL DEFAULT '',
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, discord_id)
);

CREATE TABLE mood_entries (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    member_id   BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    mood        SMALLINT NOT NULL CHECK (mood BETWEEN 1 AND 5),
    message     TEXT NOT NULL DEFAULT '' CHECK (char_length(message) <= 500),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (member_id, day)
);

CREATE INDEX mood_entries_guild_day_idx ON mood_entries (guild_id, day);
