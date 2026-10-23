DROP TABLE game_guesses;
DROP TABLE game_answers;
DROP TABLE game_rounds;
DELETE FROM dispatches WHERE kind IN ('game', 'game_guess');
ALTER TABLE dispatches DROP CONSTRAINT dispatches_kind_check;
ALTER TABLE dispatches ADD CONSTRAINT dispatches_kind_check CHECK (kind IN ('survey', 'report'));
ALTER TABLE guilds DROP COLUMN games_enabled;
