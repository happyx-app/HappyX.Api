-- survey_enabled says whether the bot sends the daily survey and the «Who's who?» game. Servers
-- that already use HappyX keep it on; servers added from now on start with it off, so nobody gets
-- a DM before the admin has gone through the setup and started the survey.
ALTER TABLE guilds ADD COLUMN survey_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE guilds ALTER COLUMN survey_enabled SET DEFAULT FALSE;
