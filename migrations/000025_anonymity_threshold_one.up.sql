-- An admin may lower the anonymity threshold down to 1, so the dashboard and the reports show a
-- day even when a single person answered. Each answer keeps the threshold it was given under,
-- so days already answered keep theirs.
ALTER TABLE guilds DROP CONSTRAINT guilds_anonymity_threshold_check;
ALTER TABLE guilds ADD CONSTRAINT guilds_anonymity_threshold_check CHECK (anonymity_threshold BETWEEN 1 AND 50);
