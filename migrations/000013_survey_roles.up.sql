-- Who receives the daily survey by Discord role: everyone ('all'), everyone except members with
-- one of survey_role_ids ('except'), or only members with one of them ('only').
ALTER TABLE guilds
    ADD COLUMN survey_roles_mode TEXT NOT NULL DEFAULT 'all' CHECK (survey_roles_mode IN ('all', 'except', 'only')),
    ADD COLUMN survey_role_ids   TEXT[] NOT NULL DEFAULT '{}';
