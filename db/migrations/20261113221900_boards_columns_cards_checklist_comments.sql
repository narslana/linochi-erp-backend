-- +goose Up
-- Boards, columns, cards, checklists and comments (TAV-01 to TAV-05)

CREATE TABLE boards (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    archived_at timestamptz,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE board_columns (
    id uuid PRIMARY KEY,
    board_id uuid NOT NULL REFERENCES boards(id),
    name text NOT NULL,
    sort_order int NOT NULL,
    archived_at timestamptz,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cards (
    id uuid PRIMARY KEY,
    board_id uuid NOT NULL REFERENCES boards(id),
    column_id uuid NOT NULL REFERENCES board_columns(id),
    title text NOT NULL,
    description_md text,
    assignee_id uuid REFERENCES users(id),
    due_date date,
    labels jsonb NOT NULL DEFAULT '[]'::jsonb,
    archived_at timestamptz,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE card_checklist_items (
    id uuid PRIMARY KEY,
    card_id uuid NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    title text NOT NULL,
    completed boolean NOT NULL DEFAULT false,
    sort_order int NOT NULL,
    archived_at timestamptz,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE card_comments (
    id uuid PRIMARY KEY,
    card_id uuid NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    body_md text NOT NULL,
    author_id uuid NOT NULL REFERENCES users(id),
    archived_at timestamptz,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX idx_board_columns_board_id ON board_columns(board_id);
CREATE INDEX idx_board_columns_board_order ON board_columns(board_id, sort_order);
CREATE INDEX idx_cards_board_id ON cards(board_id);
CREATE INDEX idx_cards_column_id ON cards(column_id);
CREATE INDEX idx_cards_assignee ON cards(assignee_id) WHERE assignee_id IS NOT NULL;
CREATE INDEX idx_cards_due_date ON cards(due_date) WHERE due_date IS NOT NULL;
CREATE INDEX idx_cards_labels ON cards USING gin(labels);
CREATE INDEX idx_card_checklist_card_order ON card_checklist_items(card_id, sort_order);
CREATE INDEX idx_card_comments_card_id ON card_comments(card_id);

-- Triggers (set_updated_at is already defined in the users migration)
CREATE TRIGGER set_updated_at_boards
    BEFORE UPDATE ON boards
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_board_columns
    BEFORE UPDATE ON board_columns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_cards
    BEFORE UPDATE ON cards
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_card_checklist_items
    BEFORE UPDATE ON card_checklist_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_card_comments
    BEFORE UPDATE ON card_comments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Audit: append-only, no DELETE (NFK-09)
CREATE TRIGGER guard_audit_boards
    BEFORE DELETE ON boards
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard('boards');

CREATE TRIGGER guard_audit_board_columns
    BEFORE DELETE ON board_columns
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard('board_columns');

CREATE TRIGGER guard_audit_cards
    BEFORE DELETE ON cards
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard('cards');

CREATE TRIGGER guard_audit_card_checklist_items
    BEFORE DELETE ON card_checklist_items
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard('card_checklist_items');

CREATE TRIGGER guard_audit_card_comments
    BEFORE DELETE ON card_comments
    FOR EACH ROW EXECUTE FUNCTION audit_log_guard('card_comments');

-- Grants to the app role
GRANT SELECT, INSERT, UPDATE ON boards TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON board_columns TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON cards TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON card_checklist_items TO linochi_erp_app;
GRANT SELECT, INSERT, UPDATE ON card_comments TO linochi_erp_app;

-- +goose Down
DROP TABLE card_comments;
DROP TABLE card_checklist_items;
DROP TABLE cards;
DROP TABLE board_columns;
DROP TABLE boards;
