-- An admin may lower the anonymity threshold down to 1, so the dashboard and the reports show a
-- day even when a single person answered. Each answer keeps the threshold it was given under,
-- so days already answered keep theirs.
ALTER TABLE guilds DROP CONSTRAINT guilds_anonymity_threshold_check;
ALTER TABLE guilds ADD CONSTRAINT guilds_anonymity_threshold_check CHECK (anonymity_threshold BETWEEN 1 AND 50);

-- Lowering the threshold counts from the next day: today's survey went out under the higher one,
-- and answers to it keep that one until then.
ALTER TABLE guilds
    ADD COLUMN anonymity_threshold_since DATE,
    ADD COLUMN anonymity_threshold_before SMALLINT;
