-- 0002_domain_description.up.sql — add Domain.description.
--
-- Surfaced by the tutor import (see docs/entities/decisions.md): specs/index.md
-- gives every domain a one-line description, which the initial schema had nowhere
-- to store. Generic (every domain may carry a description), so it belongs in core.
ALTER TABLE `domain` ADD COLUMN `description` TEXT;
