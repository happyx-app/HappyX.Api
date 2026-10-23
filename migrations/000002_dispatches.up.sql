-- Records which surveys and reports have been sent, so the bot never sends one twice
-- (e.g. after a restart) and several bot instances can't race each other.
CREATE TABLE dispatches (
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('survey', 'report')),
    period      DATE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, kind, period)
);
