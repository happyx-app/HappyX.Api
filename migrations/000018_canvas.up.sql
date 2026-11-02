-- The team canvas: members draw on the background of the server's dashboard pages. Each row is
-- one stroke; the strokes of one drawing share drawing_id, so they are selected and erased
-- together. canvas_version goes up on every change, so the dashboard can ask cheaply whether
-- there is anything new.
ALTER TABLE guilds
    ADD COLUMN canvas_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN canvas_version BIGINT NOT NULL DEFAULT 0;

CREATE TABLE canvas_strokes (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    member_id   BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    drawing_id  TEXT NOT NULL CHECK (char_length(drawing_id) BETWEEN 1 AND 36),
    color       SMALLINT NOT NULL CHECK (color BETWEEN 0 AND 4),
    -- Where the drawing is anchored, as a fraction of the screen (0 to 1).
    anchor_x    REAL NOT NULL CHECK (anchor_x BETWEEN 0 AND 1),
    anchor_y    REAL NOT NULL CHECK (anchor_y BETWEEN 0 AND 1),
    -- The points in px from the anchor: big-endian int16 pairs, 4 bytes a point.
    points      BYTEA NOT NULL CHECK (octet_length(points) BETWEEN 8 AND 2000 AND octet_length(points) % 4 = 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX canvas_strokes_guild_idx ON canvas_strokes (guild_id, id);
CREATE INDEX canvas_strokes_member_idx ON canvas_strokes (member_id, created_at);
