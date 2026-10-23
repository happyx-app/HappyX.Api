-- Remember whether each answer was given while the guild was anonymous, so turning
-- anonymity off later never reveals who wrote earlier answers. Existing answers are
-- treated as anonymous, since we cannot know what members were told when they answered.
ALTER TABLE mood_entries ADD COLUMN anonymous BOOLEAN NOT NULL DEFAULT TRUE;
