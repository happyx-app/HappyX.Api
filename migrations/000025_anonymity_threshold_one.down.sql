UPDATE guilds SET anonymity_threshold = 3 WHERE anonymity_threshold < 3;
ALTER TABLE guilds DROP CONSTRAINT guilds_anonymity_threshold_check;
ALTER TABLE guilds ADD CONSTRAINT guilds_anonymity_threshold_check CHECK (anonymity_threshold BETWEEN 3 AND 50);
