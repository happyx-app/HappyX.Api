-- Team events: something that happened to a server's team over one or more days (a hack, a
-- release, a quarter close), marked on the mood chart by whoever manages the server so a change
-- in mood can be read against it. Every member sees them. They name no one: who marked an event
-- is not kept.
CREATE TABLE team_events (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    starts_on   DATE NOT NULL,
    ends_on     DATE NOT NULL CHECK (ends_on >= starts_on),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- color is one of the dashboard's bookmark colours, by name.
    color       TEXT NOT NULL DEFAULT 'yellow' CHECK (color IN ('yellow', 'orange', 'red', 'green', 'blue', 'pink')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX team_events_guild_idx ON team_events (guild_id, starts_on);
