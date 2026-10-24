-- A guild whose bot was removed is kept for a while (left_at) in case the bot is added back,
-- then deleted with all its data. A guild deleted from the dashboard is remembered in
-- guild_removals until the bot has left the server, so it can't register again meanwhile.
ALTER TABLE guilds ADD COLUMN left_at TIMESTAMPTZ;
UPDATE guilds SET left_at = updated_at WHERE NOT active;

CREATE TABLE guild_removals (
    discord_id  TEXT PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
