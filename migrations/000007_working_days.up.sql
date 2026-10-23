-- The days the daily survey goes out, one bit per day of the week with bit 0 for Sunday
-- (Go's time.Weekday). 62 is Monday to Friday.
ALTER TABLE guilds ADD COLUMN working_days SMALLINT NOT NULL DEFAULT 62
    CHECK (working_days BETWEEN 1 AND 127);
