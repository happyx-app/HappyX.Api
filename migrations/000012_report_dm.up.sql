-- Whether the main admin gets the weekly/monthly report as a Discord DM. Off means web and /report only.
ALTER TABLE guilds ADD COLUMN report_dm BOOLEAN NOT NULL DEFAULT TRUE;
