-- The server's text and announcement channels, as the bot sees them, so the dashboard can offer
-- them for the kudos channel. Like guild_roles, the bot replaces the whole list on every change.
CREATE TABLE guild_channels (
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    discord_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL DEFAULT 0,
    can_post    BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (guild_id, discord_id)
);
