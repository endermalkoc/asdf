-- 0017_spec_path_domain_relative.up.sql — req_spec.path becomes DOMAIN-RELATIVE: the
-- leading domain-slug segment is no longer stored in the path (the domain lives only in
-- domain_id). The full location is reconstructed as `domain.slug + '/' + path` by the
-- generator and the ref resolver. This removes the domain↔path duplication (renaming a
-- domain no longer leaves a stale path prefix) and makes the top-level directory always
-- equal the domain.
--
-- A domain-relative path is no longer globally unique (e.g. each domain may have an
-- `index.md`), so the UNIQUE key moves from (path) to (domain_id, path). The path VALUES
-- are import-populated, so a re-import rewrites them to the relative form (like every
-- prior section/path migration — DDL + re-import).
-- path holds only the DIRECTORY (no filename); the filename is `slug`.md. So the full
-- location identity is (domain_id, path, slug), not (domain_id, path).
ALTER TABLE `req_spec`
    DROP INDEX `uk_spec_path`,
    ADD UNIQUE KEY `uk_spec_location` (`domain_id`, `path`, `slug`);
