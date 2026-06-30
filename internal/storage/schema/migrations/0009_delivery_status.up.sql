-- 0009_delivery_status.up.sql — the one enum value-set that earns a table.
-- requirement.delivery_status carries latent POLICY (e2e-sufficient ⇒ an e2e test;
-- shared ⇒ a shared test; not-implemented ⇒ a milestone). This lookup table moves
-- that policy from prose into data (boolean columns), so a future `cusp check` can
-- enforce it data-drivenly and so the set is configurable by adding rows.
--
-- Keyed by the business value itself (`key` is the PK, like a classic reference
-- table — a deliberate exception to the ULID-PK rule). requirement.delivery_status
-- references this `key` with NO foreign key, on purpose: the lookup is SOFT, so an
-- unknown status is tolerated and surfaced by `check`, never rejected at write
-- (matches the seed-value leniency). `key` is a reserved word → backticked.
-- See docs/entities/requirements.md#deliverystatus and docs/entities/decisions.md.
CREATE TABLE IF NOT EXISTS `delivery_status` (
    `key`                  VARCHAR(32)  NOT NULL,                  -- business value, = requirement.delivery_status
    `label`                VARCHAR(128),
    `description`          TEXT,
    `sequence`             INT          NOT NULL DEFAULT 0,        -- sort / coverage order
    `counts_as_covered`    BOOLEAN      NOT NULL DEFAULT FALSE,    -- rolls up as "done" in coverage stats
    `requires_e2e_test`    BOOLEAN      NOT NULL DEFAULT FALSE,    -- ≥1 layer=e2e TestCase
    `requires_shared_test` BOOLEAN      NOT NULL DEFAULT FALSE,    -- ≥1 layer=shared TestCase
    `requires_milestone`   BOOLEAN      NOT NULL DEFAULT FALSE,    -- milestone_id required
    `created_at`           DATETIME,
    `updated_at`           DATETIME,
    PRIMARY KEY (`key`)
);

-- Seed the 7 statuses + their policy (transcribed from requirements.md). INSERT
-- IGNORE keeps re-runs idempotent and never clobbers a row a user has customized.
INSERT IGNORE INTO `delivery_status`
    (`key`, `label`, `sequence`, `counts_as_covered`, `requires_e2e_test`, `requires_shared_test`, `requires_milestone`) VALUES
    ('covered',         'Covered',          1, TRUE,  FALSE, FALSE, FALSE),
    ('e2e-sufficient',  'E2E sufficient',   2, TRUE,  TRUE,  FALSE, FALSE),
    ('shared',          'Shared coverage',  3, TRUE,  FALSE, TRUE,  FALSE),
    ('test-pending',    'Test pending',     4, FALSE, FALSE, FALSE, FALSE),
    ('schema-only',     'Schema only',      5, FALSE, FALSE, FALSE, FALSE),
    ('deferred',        'Deferred',         6, FALSE, FALSE, FALSE, FALSE),
    ('not-implemented', 'Not implemented',  7, FALSE, FALSE, FALSE, TRUE);
