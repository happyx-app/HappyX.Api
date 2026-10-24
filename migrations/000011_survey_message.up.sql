-- The admin's own text for the daily survey, with {{placeholders}}. Empty means the default text.
ALTER TABLE guilds ADD COLUMN survey_message TEXT NOT NULL DEFAULT ''
    CHECK (char_length(survey_message) <= 500);
