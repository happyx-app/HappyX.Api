-- «Who's who?»: every week members answer an icebreaker question, then guess on the web
-- who wrote each answer. Off by default; the guild's admin turns it on.
ALTER TABLE guilds ADD COLUMN games_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE dispatches DROP CONSTRAINT dispatches_kind_check;
ALTER TABLE dispatches ADD CONSTRAINT dispatches_kind_check
    CHECK (kind IN ('survey', 'report', 'game', 'game_guess'));

CREATE TABLE game_rounds (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    week        DATE NOT NULL CHECK (extract(isodow FROM week) = 1),
    question    TEXT NOT NULL CHECK (char_length(question) BETWEEN 1 AND 300),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, week)
);

-- ref is what the web sees: a random ID that doesn't give away who answered first.
CREATE TABLE game_answers (
    id          BIGSERIAL PRIMARY KEY,
    ref         UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    round_id    BIGINT NOT NULL REFERENCES game_rounds(id) ON DELETE CASCADE,
    member_id   BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    answer      TEXT NOT NULL CHECK (char_length(answer) BETWEEN 1 AND 200),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (round_id, member_id)
);

CREATE TABLE game_guesses (
    answer_id         BIGINT NOT NULL REFERENCES game_answers(id) ON DELETE CASCADE,
    guesser_member_id BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    guessed_member_id BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (answer_id, guesser_member_id)
);
