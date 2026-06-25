-- 0006_user_story_narrative.up.sql — capture the prose body of a "prose-style"
-- user story (a paragraph instead of an "As a … I want … so that …" line, e.g.
-- "Any time a record is created … an immutable audit log entry is created").
-- ~125 stories in the tutor corpus use this style; as_a/i_want/so_that are empty
-- for them, so the body had nowhere to go. See docs/entities/decisions.md.
ALTER TABLE `user_story` ADD COLUMN `narrative` TEXT;
