DROP TABLE access_grants;
DROP TABLE guild_roles;
ALTER TABLE members DROP COLUMN in_server, DROP COLUMN role_ids;
