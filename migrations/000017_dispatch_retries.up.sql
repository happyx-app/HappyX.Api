-- A send that reached nobody (the API or Discord failed) gives its claim back, so the bot can try
-- again later that day. attempts caps the tries so a server whose members all have DMs closed
-- isn't retried forever.
ALTER TABLE dispatches
    ADD COLUMN attempts    SMALLINT NOT NULL DEFAULT 1 CHECK (attempts >= 1),
    ADD COLUMN released_at TIMESTAMPTZ;
