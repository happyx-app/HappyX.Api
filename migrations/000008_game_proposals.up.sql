-- Members propose next week's «Who's who?» question; on Monday one is drawn at random. A round
-- whose question was proposed remembers who proposed it, to reveal with the results.
CREATE TABLE game_proposals (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    week        DATE NOT NULL CHECK (extract(isodow FROM week) = 1),
    member_id   BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    question    TEXT NOT NULL CHECK (char_length(question) BETWEEN 1 AND 300),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, week, member_id)
);

ALTER TABLE game_rounds ADD COLUMN proposer_member_id BIGINT REFERENCES members(id) ON DELETE SET NULL;
