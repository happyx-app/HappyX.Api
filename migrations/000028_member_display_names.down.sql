DROP INDEX IF EXISTS members_server_name_idx;
ALTER TABLE members DROP COLUMN IF EXISTS display_name;
