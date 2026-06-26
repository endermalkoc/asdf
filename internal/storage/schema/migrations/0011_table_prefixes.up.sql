-- 0011_table_prefixes.up.sql — group the tables into layer "namespaces" via name
-- prefixes (MySQL/Dolt has no schema-within-database namespace; prefixes are the
-- conventional grouping). Prefixes: req_ (structure + requirements + authorization
-- rules + glossary), ent_ (entity layer), test_ (testing), plan_ (planning, incl.
-- milestone + delivery_status), pub_ (interop), rev_ (review). Tables already named
-- test_* keep their name; schema_migrations (the runner's cursor) is left alone.
-- Pure rename — no data or output change. FKs follow the rename automatically.
RENAME TABLE
    `domain`                 TO `req_domain`,
    `spec`                   TO `req_spec`,
    `doc_section`            TO `req_doc_section`,
    `requirement`            TO `req_requirement`,
    `requirement_group`      TO `req_requirement_group`,
    `user_story`             TO `req_user_story`,
    `acceptance_scenario`    TO `req_acceptance_scenario`,
    `edge`                   TO `req_edge`,
    `entity_ref`             TO `req_entity_ref`,
    `privilege`              TO `req_privilege`,
    `access_rule`            TO `req_access_rule`,
    `glossary_term`          TO `req_glossary_term`,
    `glossary_alias`         TO `req_glossary_alias`,
    `requirement_test_case`  TO `req_requirement_test_case`,
    `entity`                 TO `ent_entity`,
    `entity_attribute`       TO `ent_attribute`,
    `entity_relationship`    TO `ent_relationship`,
    `configuration`          TO `test_configuration`,
    `capability`             TO `plan_capability`,
    `deliverable`            TO `plan_deliverable`,
    `view`                   TO `plan_view`,
    `milestone`              TO `plan_milestone`,
    `delivery_status`        TO `plan_delivery_status`,
    `capability_milestone`   TO `plan_capability_milestone`,
    `capability_deliverable` TO `plan_capability_deliverable`,
    `deliverable_view`       TO `plan_deliverable_view`,
    `deliverable_dependency` TO `plan_deliverable_dependency`,
    `external_ref`           TO `pub_external_ref`,
    `changeset`              TO `rev_changeset`,
    `review`                 TO `rev_review`,
    `comment`                TO `rev_comment`,
    `actor`                  TO `rev_actor`;
