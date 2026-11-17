-- A replaced or removed vacation is kept for the days that had already gone by, so «My evolution»
-- can show them and streaks keep not counting them. archived marks such a vacation: each person
-- still has at most one that isn't, of each kind (their own, and one per server set by an admin).
ALTER TABLE vacations ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;

DROP INDEX vacations_scope_idx;
CREATE UNIQUE INDEX vacations_scope_idx ON vacations (user_discord_id, COALESCE(guild_id, 0)) WHERE NOT archived;
CREATE INDEX vacations_user_idx ON vacations (user_discord_id, ends_on);
