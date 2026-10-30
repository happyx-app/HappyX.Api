-- A server's answer to the Pro waitlist on the dashboard («I'm interested» and how much it
-- would pay). It belongs to the server and names no person.
CREATE TABLE pro_interest (
    guild_id    BIGINT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    interested  BOOLEAN NOT NULL DEFAULT FALSE,
    price       TEXT NOT NULL DEFAULT '' CHECK (price IN ('', '0', '5', '9', '29')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
