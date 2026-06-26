-- 0020_entity_ref_drop_kind.up.sql — req_entity_ref.kind was always 'references'.
-- entity_ref is the inline-link "references" projection of [[TYPE:key]] tokens; typed
-- relationship kinds (refines, depends_on, …) live on `edge`, not here. The column is a
-- constant with no consumer that branches on it, so drop it. The deterministic id and the
-- UNIQUE identity drop the (constant) kind component; a re-import regenerates the ids over
-- (owner, target) only (same cardinality — kind never varied).
ALTER TABLE `req_entity_ref` DROP INDEX `uk_entity_ref_identity`;
ALTER TABLE `req_entity_ref` DROP COLUMN `kind`;
ALTER TABLE `req_entity_ref`
    ADD UNIQUE KEY `uk_entity_ref_identity` (`owner_type`, `owner_id`, `target_type`, `target_id`);
