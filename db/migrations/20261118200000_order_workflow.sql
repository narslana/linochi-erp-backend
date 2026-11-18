-- +goose Up

-- WF-01 to WF-07, WF-10, ORD-03: the order workflow is versioned data. Steps and
-- transitions belong to a version and never change once published; a change
-- publishes a new version. Orders keep the version they started with.
-- The old unversioned workflow tables held free text and no data worth keeping.

-- +goose StatementBegin
DROP TABLE IF EXISTS workflow_effects;
DROP TABLE IF EXISTS workflow_conditions;
DROP TABLE IF EXISTS workflow_transitions;
DROP TABLE IF EXISTS workflow_steps;

CREATE TABLE workflows (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL,
    -- NULL is the default flow. Per-channel flows come with WF-08 (later).
    channel     text,
    version     int NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_workflows_default ON workflows ((channel IS NULL)) WHERE channel IS NULL;
CREATE UNIQUE INDEX idx_workflows_channel ON workflows (channel) WHERE channel IS NOT NULL;

-- "version" here is the number of the flow version (1, 2, ...), as the T2 brief
-- names it. A published version is immutable apart from is_active.
CREATE TABLE workflow_versions (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    workflow_id   uuid NOT NULL REFERENCES workflows(id),
    version       int NOT NULL CHECK (version > 0),
    published_at  timestamptz NOT NULL DEFAULT now(),
    is_active     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, version)
);
CREATE UNIQUE INDEX idx_workflow_versions_active ON workflow_versions (workflow_id) WHERE is_active;

-- Labels are not stored: the web maps key to sv.json / en.json.
-- is_start: where an order begins (exactly one per version).
-- is_final: no normal transition leaves it.
-- is_locked: nothing leaves it, not even an admin override (delivered).
-- stock_phase: what stock looks like for an order in this step; the engine
-- derives the effects of an override from the difference between two phases.
CREATE TABLE workflow_steps (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    version_id  uuid NOT NULL REFERENCES workflow_versions(id),
    key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*$'),
    sort_order  int NOT NULL,
    is_start    boolean NOT NULL DEFAULT false,
    is_final    boolean NOT NULL DEFAULT false,
    is_locked   boolean NOT NULL DEFAULT false CHECK (NOT is_locked OR is_final),
    stock_phase text NOT NULL DEFAULT 'none' CHECK (stock_phase IN ('none', 'reserved', 'shipped')),
    version     int NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version_id, key),
    UNIQUE (id, version_id)
);

CREATE TABLE workflow_transitions (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    version_id    uuid NOT NULL REFERENCES workflow_versions(id),
    from_step_id  uuid NOT NULL,
    to_step_id    uuid NOT NULL,
    effect        text CHECK (effect IN ('reserve', 'release', 'ship')),
    version       int NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version_id, from_step_id, to_step_id),
    FOREIGN KEY (from_step_id, version_id) REFERENCES workflow_steps (id, version_id),
    FOREIGN KEY (to_step_id, version_id) REFERENCES workflow_steps (id, version_id)
);

GRANT SELECT, INSERT, UPDATE ON workflows, workflow_versions TO linochi_erp_app;
GRANT SELECT, INSERT ON workflow_steps, workflow_transitions TO linochi_erp_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- The default flow, version 1 (WF-02, WF-03).
DO $seed$
DECLARE
    wf uuid := uuidv7();
    v  uuid := uuidv7();
    r  record;
BEGIN
    INSERT INTO workflows (id, name, channel) VALUES (wf, 'Default order flow', NULL);
    INSERT INTO workflow_versions (id, workflow_id, version, is_active) VALUES (v, wf, 1, true);

    INSERT INTO workflow_steps (version_id, key, sort_order, is_start, is_final, is_locked, stock_phase) VALUES
        (v, 'new',           10,  true,  false, false, 'none'),
        (v, 'in_review',     20,  false, false, false, 'none'),
        (v, 'accepted',      30,  false, false, false, 'reserved'),
        (v, 'picking',       40,  false, false, false, 'reserved'),
        (v, 'packing',       50,  false, false, false, 'reserved'),
        (v, 'label_printed', 60,  false, false, false, 'reserved'),
        (v, 'ready_to_ship', 70,  false, false, false, 'reserved'),
        (v, 'posted',        80,  false, false, false, 'shipped'),
        (v, 'in_transit',    90,  false, false, false, 'shipped'),
        (v, 'delivered',     100, false, true,  true,  'shipped'),
        (v, 'rejected',      110, false, true,  false, 'none'),
        (v, 'cancelled',     120, false, true,  false, 'none');

    -- Each step to the next.
    FOR r IN
        SELECT s.id AS from_id, n.id AS to_id, n.key AS to_key
        FROM workflow_steps s
        JOIN workflow_steps n ON n.version_id = s.version_id
         AND n.sort_order = (SELECT min(x.sort_order) FROM workflow_steps x
                             WHERE x.version_id = s.version_id AND x.sort_order > s.sort_order)
        WHERE s.version_id = v AND NOT s.is_final
    LOOP
        INSERT INTO workflow_transitions (version_id, from_step_id, to_step_id, effect)
        VALUES (v, r.from_id, r.to_id,
                CASE r.to_key WHEN 'accepted' THEN 'reserve' WHEN 'posted' THEN 'ship' END);
    END LOOP;

    -- new and in_review can be rejected.
    INSERT INTO workflow_transitions (version_id, from_step_id, to_step_id, effect)
    SELECT v, f.id, t.id, NULL
    FROM workflow_steps f, workflow_steps t
    WHERE f.version_id = v AND t.version_id = v AND f.key IN ('new', 'in_review') AND t.key = 'rejected';

    -- Any non-final step before posted can be cancelled; a reserved one releases its stock.
    INSERT INTO workflow_transitions (version_id, from_step_id, to_step_id, effect)
    SELECT v, f.id, t.id, CASE WHEN f.stock_phase = 'reserved' THEN 'release' END
    FROM workflow_steps f, workflow_steps t
    WHERE f.version_id = v AND t.version_id = v AND t.key = 'cancelled'
      AND NOT f.is_final AND f.sort_order < (SELECT sort_order FROM workflow_steps WHERE version_id = v AND key = 'posted');
END
$seed$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The active version of the default flow: what a new order starts in.
CREATE FUNCTION default_workflow_version_id() RETURNS uuid
LANGUAGE sql STABLE AS $$
    SELECT v.id FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
    WHERE w.channel IS NULL AND v.is_active
$$;

ALTER TABLE orders ADD COLUMN workflow_version_id uuid REFERENCES workflow_versions(id);
-- Orders from before the workflow: any status that is not a step key starts again at new.
UPDATE orders SET workflow_version_id = default_workflow_version_id();
UPDATE orders SET status = 'new'
WHERE status NOT IN (SELECT key FROM workflow_steps WHERE version_id = workflow_version_id);
ALTER TABLE orders ALTER COLUMN workflow_version_id SET NOT NULL;
ALTER TABLE orders ALTER COLUMN workflow_version_id SET DEFAULT default_workflow_version_id();
ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'new';
CREATE INDEX idx_orders_workflow_version ON orders(workflow_version_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- WF-10: every status change, including overrides. Time in step is computed from it.
CREATE TABLE order_status_history (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    order_id    uuid NOT NULL REFERENCES orders(id),
    from_key    text NOT NULL,
    to_key      text NOT NULL,
    -- A user id, later possibly an API key id: no foreign key.
    actor_id    uuid NOT NULL,
    reason      text NOT NULL DEFAULT '',
    is_override boolean NOT NULL DEFAULT false,
    version     int NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_status_history_order ON order_status_history (order_id, created_at, id);
GRANT SELECT, INSERT ON order_status_history TO linochi_erp_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS order_status_history;
ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'draft';
ALTER TABLE orders DROP COLUMN IF EXISTS workflow_version_id;
DROP FUNCTION IF EXISTS default_workflow_version_id();
DROP TABLE IF EXISTS workflow_transitions;
DROP TABLE IF EXISTS workflow_steps;
DROP TABLE IF EXISTS workflow_versions;
DROP TABLE IF EXISTS workflows;

CREATE TABLE workflow_steps (
    id uuid PRIMARY KEY DEFAULT uuidv7(), key text NOT NULL, name text NOT NULL,
    name_en text NOT NULL DEFAULT '', sort_order int NOT NULL DEFAULT 0,
    is_start boolean NOT NULL DEFAULT false, is_end boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false, archived_at timestamptz,
    version int NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_workflowsteps_key ON workflow_steps(key) WHERE archived = false;
CREATE TABLE workflow_transitions (
    id uuid PRIMARY KEY DEFAULT uuidv7(), step_key text NOT NULL, from_key text NOT NULL,
    to_key text NOT NULL, label text NOT NULL, label_en text NOT NULL DEFAULT '',
    sort_order int NOT NULL DEFAULT 0, archived boolean NOT NULL DEFAULT false, archived_at timestamptz,
    version int NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_workflowtransitions_step ON workflow_transitions(step_key) WHERE archived = false;
CREATE TABLE workflow_conditions (
    id uuid PRIMARY KEY DEFAULT uuidv7(), transition_id uuid NOT NULL REFERENCES workflow_transitions(id),
    kind text NOT NULL, config jsonb NOT NULL DEFAULT '{}', archived boolean NOT NULL DEFAULT false,
    archived_at timestamptz, version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE workflow_effects (
    id uuid PRIMARY KEY DEFAULT uuidv7(), transition_id uuid NOT NULL REFERENCES workflow_transitions(id),
    kind text NOT NULL, config jsonb NOT NULL DEFAULT '{}', archived boolean NOT NULL DEFAULT false,
    archived_at timestamptz, version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON workflow_steps, workflow_transitions, workflow_conditions, workflow_effects TO linochi_erp_app;
-- +goose StatementEnd
