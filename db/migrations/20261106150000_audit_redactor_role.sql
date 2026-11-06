-- +goose Up

-- Hardens the GDPR redaction of the change log (INS-05, KUN-07).
--
-- Before: audit_redact_personal_data() was owned by the table owner, and the
-- trigger let a change through when the owner ran it from a SECURITY DEFINER
-- function. That also let any other SECURITY DEFINER function the owner wrote
-- rewrite old audit rows.
--
-- Now the function is owned by a role of its own, linochi_erp_audit_redactor. It
-- cannot log in and it can UPDATE three columns of one table, nothing else. The
-- trigger lets through exactly that role (current_user), and nobody else, the
-- owner included.
--
-- The role must exist before this runs, and the owner must be a member of it
-- (WITH SET TRUE, WITH INHERIT FALSE) because ALTER FUNCTION ... OWNER TO needs
-- that. H1 creates the role, Docker Compose db/dev/initdb and the tests
-- internal/platform/testdb. The app role is never a member.

-- What the redactor needs: to find the rows (SELECT) and to replace values in
-- three columns. It does not get INSERT, DELETE or TRUNCATE, nor UPDATE on the
-- other columns. updated_at and version are set by triggers, which needs no
-- privilege on them.
GRANT SELECT ON audit_log TO linochi_erp_audit_redactor;
GRANT UPDATE (before, after, redacted_at) ON audit_log TO linochi_erp_audit_redactor;

-- The trigger now names the role instead of "the owner, called through a definer
-- function". It also counts the redaction in version, so the function needs no
-- privilege on that column.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_log_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP IN ('DELETE', 'TRUNCATE') THEN
        RAISE EXCEPTION 'audit_log is append-only: rows are never deleted'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    IF current_user <> 'linochi_erp_audit_redactor' THEN
        RAISE EXCEPTION 'audit_log is append-only: only the role linochi_erp_audit_redactor may change old rows'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    IF (to_jsonb(NEW) - ARRAY['before', 'after', 'redacted_at', 'updated_at', 'version'])
       IS DISTINCT FROM
       (to_jsonb(OLD) - ARRAY['before', 'after', 'redacted_at', 'updated_at', 'version']) THEN
        RAISE EXCEPTION 'audit_log is append-only: only before, after and redacted_at may change'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- Same function as before, minus the version update (the trigger does that now).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_redact_personal_data(p_entity_type text, p_entity_id uuid, p_fields text[])
RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public AS $$
DECLARE
    n integer;
BEGIN
    IF p_fields IS NULL OR cardinality(p_fields) = 0 THEN
        RAISE EXCEPTION 'audit_redact_personal_data needs at least one field';
    END IF;
    UPDATE audit_log
    SET before      = audit_redact_keys(before, p_fields),
        after       = audit_redact_keys(after, p_fields),
        redacted_at = now()
    WHERE entity_type = p_entity_type
      AND entity_id = p_entity_id
      AND (before ?| p_fields OR after ?| p_fields);
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END;
$$;
-- +goose StatementEnd

-- Hand the function over. The new owner must have CREATE on the schema while
-- this happens; it is taken back right after, so the role can create nothing.
-- EXECUTE stays revoked from PUBLIC and granted to the app role only, and the
-- owner, which no longer owns the function, cannot call it.
--
-- To change the function in a later migration: SET LOCAL ROLE
-- linochi_erp_audit_redactor first, and RESET ROLE before the migration ends.
GRANT CREATE ON SCHEMA public TO linochi_erp_audit_redactor;
ALTER FUNCTION audit_redact_personal_data(text, uuid, text[]) OWNER TO linochi_erp_audit_redactor;
REVOKE CREATE ON SCHEMA public FROM linochi_erp_audit_redactor;

-- +goose Down

-- Back to the state of 20261106140100_audit_log.sql: the owner owns the function
-- and the trigger lets through an owner that is called through a definer function.
-- The redactor drops the function it owns, then the owner recreates the old one.
SET LOCAL ROLE linochi_erp_audit_redactor;
DROP FUNCTION audit_redact_personal_data(text, uuid, text[]);
RESET ROLE;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_log_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    table_owner name;
BEGIN
    IF TG_OP IN ('DELETE', 'TRUNCATE') THEN
        RAISE EXCEPTION 'audit_log is append-only: rows are never deleted'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    SELECT pg_get_userbyid(relowner) INTO table_owner FROM pg_class WHERE oid = TG_RELID;
    IF current_user <> table_owner OR session_user = current_user THEN
        RAISE EXCEPTION 'audit_log is append-only: only audit_redact_personal_data() may change old rows'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    IF (to_jsonb(NEW) - ARRAY['before', 'after', 'redacted_at', 'updated_at', 'version'])
       IS DISTINCT FROM
       (to_jsonb(OLD) - ARRAY['before', 'after', 'redacted_at', 'updated_at', 'version']) THEN
        RAISE EXCEPTION 'audit_log is append-only: only before, after and redacted_at may change'
            USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION audit_redact_personal_data(p_entity_type text, p_entity_id uuid, p_fields text[])
RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public AS $$
DECLARE
    n integer;
BEGIN
    IF p_fields IS NULL OR cardinality(p_fields) = 0 THEN
        RAISE EXCEPTION 'audit_redact_personal_data needs at least one field';
    END IF;
    UPDATE audit_log
    SET before      = audit_redact_keys(before, p_fields),
        after       = audit_redact_keys(after, p_fields),
        redacted_at = now(),
        version     = version + 1
    WHERE entity_type = p_entity_type
      AND entity_id = p_entity_id
      AND (before ?| p_fields OR after ?| p_fields);
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION audit_redact_personal_data(text, uuid, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_redact_personal_data(text, uuid, text[]) TO linochi_erp_app;

REVOKE UPDATE (before, after, redacted_at) ON audit_log FROM linochi_erp_audit_redactor;
REVOKE SELECT ON audit_log FROM linochi_erp_audit_redactor;
