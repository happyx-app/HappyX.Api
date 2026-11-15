-- What HappyX tells the team in a channel: what the admins say they'll do after a report, and a
-- short summary of the week every Monday. Both go to the team channel, or to the kudos channel
-- while it isn't set.
ALTER TABLE guilds
    ADD COLUMN team_channel_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN team_summary BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE dispatches DROP CONSTRAINT dispatches_kind_check;
ALTER TABLE dispatches ADD CONSTRAINT dispatches_kind_check
    CHECK (kind IN ('survey', 'report', 'game', 'game_guess', 'team_summary'));

-- A message an admin published in a channel about a report: what they saw and what they'll do.
-- posted_on is the day it was published, in the guild's timezone: the next survey day's thanks
-- card points to it.
CREATE TABLE team_notes (
    id               BIGSERIAL PRIMARY KEY,
    guild_id         BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    author_member_id BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    period_from      DATE NOT NULL,
    period_to        DATE NOT NULL CHECK (period_to >= period_from),
    body             TEXT NOT NULL,
    channel_id       TEXT NOT NULL,
    message_id       TEXT NOT NULL,
    posted_on        DATE NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX team_notes_guild_idx ON team_notes (guild_id, posted_on);
