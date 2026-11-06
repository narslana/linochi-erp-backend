-- +goose Up

-- The change log (INS-05): who or which API key changed what and when, with the
-- values before and after. Rows are written in the same transaction as the
-- change (internal/audit) and are never updated or deleted (NFK-09), with one
-- exception: audit_redact_personal_data() below, for GDPR (KUN-07).
--
-- before / after hold only the fields that changed, except that a create stores
-- the whole record in `after` and an archive the whole record in `before`.
CREATE TABLE audit_log (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    version     integer     NOT NULL DEFAULT 1,
    actor_type  text        NOT NULL,
    actor_id    uuid,
    -- Names a system actor, for example "oidc-login". Never a person's name.
    actor_label text        NOT NULL DEFAULT '',
    module      text        NOT NULL,
    entity_type text        NOT NULL,
    entity_id   uuid        NOT NULL,
    action      text        NOT NULL,
    before      jsonb,
    after       jsonb,
    -- For example the reason of a workflow override (WF-05).
    reason      text,
    trace_id    text,
    -- Set when personal data in this row has been redacted.
    redacted_at timestamptz,
    CONSTRAINT audit_log_actor_type_check CHECK (actor_type IN ('user', 'api_key', 'system')),
    CONSTRAINT audit_log_actor_check CHECK (
        (actor_type = 'system' AND actor_id IS NULL AND actor_label <> '')
        OR (actor_type <> 'system' AND actor_id IS NOT NULL)
    )
);

-- The later filters on record, user, module and date.
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_actor_idx  ON audit_log (actor_id, created_at DESC);
CREATE INDEX audit_log_module_idx ON audit_log (module, created_at DESC);
CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);

-- Append-only. The app role has no UPDATE, DELETE or TRUNCATE privilege, and this
-- trigger is a second lock that also stops the owner, connected directly.
--
-- The one way through is audit_redact_personal_data(), which is SECURITY DEFINER.
-- Inside it current_user is the table owner while session_user is still the role
-- that logged in, so "current_user is the owner but session_user is someone else"
-- identifies a call through such a function. There is no session flag that code
-- could set to get past it, and the app role cannot SET ROLE to the owner.
-- Even then only before, after, redacted_at, updated_at and version may change.
-- +goose StatementBegin
CREATE FUNCTION audit_log_guard() RETURNS trigger
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

CREATE TRIGGER audit_log_guard
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard();
CREATE TRIGGER audit_log_guard_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_guard();
-- Runs after the guard (triggers fire in name order).
CREATE TRIGGER audit_log_set_updated_at
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Replaces the values of the given top-level keys with "[redacted]".
-- +goose StatementBegin
CREATE FUNCTION audit_redact_keys(doc jsonb, keys text[]) RETURNS jsonb
LANGUAGE sql IMMUTABLE
SET search_path = pg_catalog AS $$
    SELECT CASE WHEN doc IS NULL THEN NULL ELSE (
        SELECT coalesce(
            jsonb_object_agg(e.key, CASE WHEN e.key = ANY (keys) THEN to_jsonb('[redacted]'::text) ELSE e.value END),
            '{}'::jsonb)
        FROM jsonb_each(doc) AS e
    ) END
$$;
-- +goose StatementEnd

-- GDPR (KUN-07): replaces the old values of personal fields of one record in
-- the audit log and returns how many rows changed. internal/audit calls it and
-- writes an audit row about the redaction itself.
--
-- SECURITY DEFINER hygiene: search_path is pinned, EXECUTE is revoked from
-- PUBLIC and granted to the app role only.
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
-- Insert and read only: no UPDATE, DELETE or TRUNCATE.
GRANT SELECT, INSERT ON audit_log TO linochi_erp_app;

-- +goose Down
DROP TABLE audit_log;
DROP FUNCTION audit_redact_personal_data(text, uuid, text[]);
DROP FUNCTION audit_redact_keys(jsonb, text[]);
DROP FUNCTION audit_log_guard();
