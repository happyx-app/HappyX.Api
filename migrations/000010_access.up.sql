-- Who may see a guild's dashboard besides its main admin and Discord Administrators: users or
-- roles granted "view" (reports) or "manage" (reports and settings). The bot keeps the guild's
-- roles and each member's roles in sync so grants to a role can be checked and picked.
ALTER TABLE members
    ADD COLUMN role_ids  TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN in_server BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE guild_roles (
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    discord_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    color       INTEGER NOT NULL DEFAULT 0,
    position    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (guild_id, discord_id)
);

CREATE TABLE access_grants (
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('user', 'role')),
    target_id   TEXT NOT NULL,
    level       TEXT NOT NULL CHECK (level IN ('view', 'manage')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, kind, target_id)
);

CREATE INDEX access_grants_target_idx ON access_grants (target_id);
