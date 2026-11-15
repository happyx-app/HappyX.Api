ALTER TABLE mood_entries
    DROP COLUMN anonymity_threshold;
ALTER TABLE guilds
    DROP COLUMN named_since,
    DROP COLUMN anonymity_threshold;
