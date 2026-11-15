-- Messages HappyX keeps in a channel and edits instead of posting again, one of each kind per
-- guild. 'survey' is the daily survey for whoever doesn't get HappyX's DMs: a button there
-- answers it privately.
CREATE TABLE channel_posts (
    guild_id   BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('survey')),
    channel_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, kind)
);
