-- 0012_drop_authz_layer.up.sql
--
-- Remove the authorization layer (`req_privilege` + `req_access_rule`).
--
-- Both tables were modeled from the tutor corpus's CASL-style row-level access
-- model — a *specific* authorization paradigm ((resource, scope, action) triples,
-- entity-attached rules, "never role names") baked into the core schema, against
-- the CLAUDE.md "keep the core generic" invariant. Neither is consumed: nothing
-- reads `req_access_rule` at all, and `req_privilege` is write-only (the tutor
-- importer fills it; no command or generator reads it). The access content humans
-- see is rendered from the `row_level_access` doc_section prose, not these tables,
-- so generated output is unaffected. A generic authorization concept can return
-- later, on a real consumer — see ROADMAP.
--
-- Drop the referencing table (`req_access_rule`) before the referenced one
-- (`req_privilege`); `ent_entity` is referenced by `req_access_rule` too but is
-- not dropped.
DROP TABLE IF EXISTS `req_access_rule`;
DROP TABLE IF EXISTS `req_privilege`;
