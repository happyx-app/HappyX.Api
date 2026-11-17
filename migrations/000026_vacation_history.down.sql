DROP INDEX vacations_user_idx;
DROP INDEX vacations_scope_idx;
DELETE FROM vacations WHERE archived;
ALTER TABLE vacations DROP COLUMN archived;
CREATE UNIQUE INDEX vacations_scope_idx ON vacations (user_discord_id, COALESCE(guild_id, 0));
