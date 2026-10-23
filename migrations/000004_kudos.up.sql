-- Kudos: public thanks from one member to another, shown in the guild and counted in reports.
ALTER TABLE guilds ADD COLUMN kudos_channel_id TEXT NOT NULL DEFAULT '';

CREATE TABLE kudos (
    id                  BIGSERIAL PRIMARY KEY,
    guild_id            BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    giver_member_id     BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    receiver_member_id  BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    day                 DATE NOT NULL,
    message             TEXT NOT NULL CHECK (char_length(message) BETWEEN 1 AND 300),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (giver_member_id <> receiver_member_id)
);

CREATE INDEX kudos_guild_day_idx ON kudos (guild_id, day);
CREATE INDEX kudos_giver_day_idx ON kudos (giver_member_id, day);
