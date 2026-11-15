-- Anonymity that holds in small teams: the fewest people who must have answered before a
-- report shows averages, moods or comments of anonymous answers, and the day from which
-- answers carry names once anonymity is turned off (the next day, never the same one).
ALTER TABLE guilds
    ADD COLUMN anonymity_threshold SMALLINT NOT NULL DEFAULT 3
        CHECK (anonymity_threshold BETWEEN 3 AND 50),
    ADD COLUMN named_since DATE;

-- Servers that are already named have been so since before today.
UPDATE guilds SET named_since = DATE '2000-01-01' WHERE NOT anonymous;

-- Each answer keeps the threshold in force when it was given, so lowering the threshold never
-- shows a day that was answered under a higher one. Earlier answers had no threshold at all.
ALTER TABLE mood_entries
    ADD COLUMN anonymity_threshold SMALLINT NOT NULL DEFAULT 3;
