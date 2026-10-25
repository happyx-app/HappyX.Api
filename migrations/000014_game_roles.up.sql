-- Who plays «Who's who?» by Discord role, like survey_roles_mode/survey_role_ids for the survey.
ALTER TABLE guilds
    ADD COLUMN game_roles_mode TEXT NOT NULL DEFAULT 'all' CHECK (game_roles_mode IN ('all', 'except', 'only')),
    ADD COLUMN game_role_ids   TEXT[] NOT NULL DEFAULT '{}';
