-- 0001_init.down.sql — drop the initial Cusp schema.
-- Tables are dropped dependents-first so foreign keys never block a drop.

DROP TABLE IF EXISTS `deliverable_dependency`;
DROP TABLE IF EXISTS `deliverable_view`;
DROP TABLE IF EXISTS `capability_deliverable`;
DROP TABLE IF EXISTS `capability_milestone`;
DROP TABLE IF EXISTS `test_run_configuration`;
DROP TABLE IF EXISTS `requirement_test_case`;

DROP TABLE IF EXISTS `comment`;
DROP TABLE IF EXISTS `review`;
DROP TABLE IF EXISTS `changeset`;
DROP TABLE IF EXISTS `actor`;

DROP TABLE IF EXISTS `external_ref`;
DROP TABLE IF EXISTS `edge`;

DROP TABLE IF EXISTS `access_rule`;
DROP TABLE IF EXISTS `privilege`;
DROP TABLE IF EXISTS `entity_relationship`;
DROP TABLE IF EXISTS `entity_attribute`;
DROP TABLE IF EXISTS `entity`;

DROP TABLE IF EXISTS `view`;
DROP TABLE IF EXISTS `deliverable`;
DROP TABLE IF EXISTS `capability`;

DROP TABLE IF EXISTS `test_result`;
DROP TABLE IF EXISTS `test_step`;
DROP TABLE IF EXISTS `test_case`;
DROP TABLE IF EXISTS `test_run`;
DROP TABLE IF EXISTS `configuration`;
DROP TABLE IF EXISTS `test_suite`;

DROP TABLE IF EXISTS `acceptance_scenario`;
DROP TABLE IF EXISTS `user_story`;
DROP TABLE IF EXISTS `requirement`;
DROP TABLE IF EXISTS `milestone`;

DROP TABLE IF EXISTS `spec`;
DROP TABLE IF EXISTS `domain`;
