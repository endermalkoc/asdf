-- 0017_spec_path_domain_relative.down.sql — restore the global UNIQUE(path). Re-import
-- with full (domain-prefixed) paths after rolling back, since relative paths are not
-- globally unique (an `index.md` under two domains would violate uk_spec_path).
ALTER TABLE `req_spec`
    DROP INDEX `uk_spec_location`,
    ADD UNIQUE KEY `uk_spec_path` (`path`);
