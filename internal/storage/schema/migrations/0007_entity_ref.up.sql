-- 0007_entity_ref.up.sql — the queryable projection of inline cross-reference
-- links. An author/agent writes a `[[TYPE:key]]` token in a text field; at
-- ingestion it is validated, kept verbatim in the text (for `generate` to render
-- as a link), and projected into an `entity_ref` row (owner → target). This is
-- ALSO the home for the importer's prose-derived mentions (inline FR citations,
-- Key Entities lists). `edge` stays for hand-authored / structured relationships;
-- `impact`/`check` read both. See docs/entities/requirements.md#entityref and
-- docs/entities/decisions.md.
--
-- Like `edge`, the surrogate `id` is derived deterministically (uuidv5) from the
-- UNIQUE identity so the same ref created on two branches converges on merge.
CREATE TABLE IF NOT EXISTS `entity_ref` (
    `id`          VARCHAR(255) NOT NULL,                  -- deterministic over the UNIQUE identity below
    `owner_type`  VARCHAR(32)  NOT NULL,                  -- values: domain|spec|requirement|user_story|entity|milestone
    `owner_id`    VARCHAR(255) NOT NULL,
    `target_type` VARCHAR(32)  NOT NULL,                  -- values: domain|spec|requirement|entity|milestone
    `target_id`   VARCHAR(255) NOT NULL,
    `kind`        VARCHAR(32)  NOT NULL,                  -- values: references
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_entity_ref_identity` (`owner_type`, `owner_id`, `target_type`, `target_id`, `kind`),
    INDEX `idx_entity_ref_owner` (`owner_type`, `owner_id`),
    INDEX `idx_entity_ref_target` (`target_type`, `target_id`)
);
