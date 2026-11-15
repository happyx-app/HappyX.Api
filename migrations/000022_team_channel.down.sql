DROP TABLE team_notes;
DELETE FROM dispatches WHERE kind = 'team_summary';
ALTER TABLE dispatches DROP CONSTRAINT dispatches_kind_check;
ALTER TABLE dispatches ADD CONSTRAINT dispatches_kind_check
    CHECK (kind IN ('survey', 'report', 'game', 'game_guess'));
ALTER TABLE guilds DROP COLUMN team_channel_id, DROP COLUMN team_summary;
