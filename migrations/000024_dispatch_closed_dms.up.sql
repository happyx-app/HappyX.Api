-- Of the DMs that failed, how many because the person has DMs from the server closed. The admin
-- is told, so they can choose a team channel where those people answer the survey instead.
ALTER TABLE dispatches
    ADD COLUMN closed_dms INTEGER CHECK (closed_dms >= 0 AND closed_dms <= failed);
