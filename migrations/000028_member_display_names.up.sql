-- display_name is the name the member's Discord server shows for them: their server nickname,
-- else their Discord display name, else their username. The bot reports it with every member
-- sync. Empty until the bot first reports it, and the API falls back to the username then.
ALTER TABLE members ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

-- Two members of a server going by the same name are told apart by their usernames; this finds
-- the namesakes.
CREATE INDEX members_server_name_idx ON members (guild_id, lower(COALESCE(NULLIF(display_name, ''), username)));
