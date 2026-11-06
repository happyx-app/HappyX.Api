-- Vacations: days when someone gets no survey or game, and that don't count against their
-- participation or streak. guild_id NULL is the person's own, for every server they are in;
-- otherwise someone who manages that server set it for them. One of each per person.
CREATE TABLE vacations (
    id                BIGSERIAL PRIMARY KEY,
    user_discord_id   TEXT NOT NULL,
    guild_id          BIGINT REFERENCES guilds(id) ON DELETE CASCADE,
    starts_on         DATE NOT NULL,
    ends_on           DATE NOT NULL CHECK (ends_on >= starts_on),
    set_by_discord_id TEXT NOT NULL DEFAULT '',
    -- notify_pending: an admin set it and the bot hasn't told the person by DM yet.
    notify_pending    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX vacations_scope_idx ON vacations (user_discord_id, COALESCE(guild_id, 0));
CREATE INDEX vacations_guild_idx ON vacations (guild_id, ends_on);
CREATE INDEX vacations_notify_idx ON vacations (id) WHERE notify_pending;
