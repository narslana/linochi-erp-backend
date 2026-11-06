-- +goose Up

-- Two database roles (DECISIONS.md): linochi_erp_owner owns the schema and runs
-- these migrations, linochi_erp_app is what the running app connects as. The app
-- role gets exactly the rights it needs and nothing else: no DELETE (data is kept
-- forever, NFK-09), no DDL, and it does not own any table.
--
-- The role must exist before this runs. In the cluster the DatabaseRole objects
-- of H1 create both roles; in Docker Compose db/dev/initdb does, and in tests
-- internal/platform/testdb. If it is missing this migration fails with
-- "role does not exist", which is the clear signal that the environment is not
-- set up.
--
-- Every later migration grants on its own tables, explicitly and per table.
GRANT USAGE ON SCHEMA public TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON users TO linochi_erp_app;
-- Readiness reads the schema version (db.ReadinessCheck).
GRANT SELECT ON goose_db_version TO linochi_erp_app;

-- +goose Down
REVOKE SELECT ON goose_db_version FROM linochi_erp_app;
REVOKE SELECT, INSERT, UPDATE ON users FROM linochi_erp_app;
REVOKE USAGE ON SCHEMA public FROM linochi_erp_app;
