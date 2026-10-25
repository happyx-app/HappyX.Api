-- For the owner's admin dashboard. The bot reports how many people each send went to and how many
-- DMs failed (usually DMs closed), which gives the response rate and the failed sends. Guilds that
-- are deleted leave only a dated line here, with nothing that tells which server it was.
ALTER TABLE dispatches
    ADD COLUMN recipients  INTEGER CHECK (recipients >= 0),
    ADD COLUMN failed      INTEGER CHECK (failed >= 0 AND failed <= recipients),
    ADD COLUMN finished_at TIMESTAMPTZ;

CREATE TABLE guild_deletions (
    id          BIGSERIAL PRIMARY KEY,
    reason      TEXT NOT NULL CHECK (reason IN ('dashboard', 'retention')),
    deleted_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
