//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const insertAudit = `
	INSERT INTO audit_log (actor_type, actor_id, actor_label, module, entity_type, entity_id, action, before, after)
	VALUES ($1, $2, $3, 'test', 'widget', $4::uuid, 'update', $5::jsonb, $6::jsonb)`

const (
	someUUID  = "0194fa5e-0000-7000-8000-000000000001"
	otherUUID = "0194fa5e-0000-7000-8000-000000000002"
)

func addAudit(t *testing.T, conn *pgx.Conn, entity, before, after string) {
	t.Helper()
	var b, a any
	if before != "" {
		b = before
	}
	if after != "" {
		a = after
	}
	if _, err := conn.Exec(context.Background(), insertAudit, "system", nil, "test", entity, b, a); err != nil {
		t.Fatalf("insert audit row: %v", err)
	}
}

func TestINS05_AuditLogChecksTheActor(t *testing.T) {
	ctx := context.Background()
	app := appConn(t, migrated(t))

	tests := []struct {
		name                 string
		actorType, id, label any
		wantErr              bool
	}{
		{"system with a label", "system", nil, "oidc-login", false},
		{"user with an id", "user", someUUID, "", false},
		{"api key with an id", "api_key", someUUID, "", false},
		{"unknown actor type", "robot", nil, "x", true},
		{"system without a label", "system", nil, "", true},
		{"system with an id", "system", someUUID, "x", true},
		{"user without an id", "user", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.Exec(ctx, insertAudit, tt.actorType, tt.id, tt.label, someUUID, nil, `{"a":1}`)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNFK09_AppCannotUpdateDeleteOrTruncateAuditRows(t *testing.T) {
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	mustFailWith(t, "UPDATE", app, `UPDATE audit_log SET action = 'tampered'`, "permission denied")
	mustFailWith(t, "DELETE", app, `DELETE FROM audit_log`, "permission denied")
	mustFailWith(t, "TRUNCATE", app, `TRUNCATE audit_log`, "permission denied")
	mustFailWith(t, "DROP", app, `DROP TABLE audit_log`, "must be owner")
}

// The trigger is a second lock behind the privileges: even the owner, connected
// directly, cannot rewrite history. Only audit_redact_personal_data() can.
func TestNFK09_TriggerBlocksEvenTheOwnerOutsideTheDefinerFunction(t *testing.T) {
	owner := migrated(t)
	ownerConn := connect(t, owner)
	addAudit(t, appConn(t, owner), someUUID, "", `{"email":"a@example.se"}`)

	mustFailWith(t, "owner UPDATE of a locked column", ownerConn, `UPDATE audit_log SET action = 'tampered'`, "append-only")
	mustFailWith(t, "owner UPDATE of a redactable column", ownerConn, `UPDATE audit_log SET after = '{}'`, "append-only")
	mustFailWith(t, "owner DELETE", ownerConn, `DELETE FROM audit_log`, "append-only")
	mustFailWith(t, "owner TRUNCATE", ownerConn, `TRUNCATE audit_log`, "append-only")
}

func TestINS05_RedactReplacesOldValuesAndLeavesTheRest(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, `{"email":"old@example.se","display_name":"Anna","color":"red"}`, `{"email":"new@example.se","display_name":"Anna S","color":"blue"}`)
	addAudit(t, app, otherUUID, "", `{"email":"someone-else@example.se"}`)

	var n int
	if err := app.QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY['email','display_name'])`, someUUID).Scan(&n); err != nil {
		t.Fatalf("redact as the app role: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows redacted = %d, want 1", n)
	}

	var before, after string
	var redacted bool
	var version int
	if err := app.QueryRow(ctx, `SELECT before::text, after::text, redacted_at IS NOT NULL, version FROM audit_log WHERE entity_id = $1::uuid`, someUUID).Scan(&before, &after, &redacted, &version); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"old@example.se", "new@example.se", "Anna"} {
		if strings.Contains(before+after, leaked) {
			t.Errorf("%q survived the redaction: before=%s after=%s", leaked, before, after)
		}
	}
	if !strings.Contains(before, `"color": "red"`) || !strings.Contains(after, `"color": "blue"`) {
		t.Errorf("a field that was not asked for changed: before=%s after=%s", before, after)
	}
	if strings.Count(before+after, "[redacted]") != 4 {
		t.Errorf("want four [redacted] values: before=%s after=%s", before, after)
	}
	if !redacted || version != 2 {
		t.Errorf("redacted_at set = %v, version = %d (want true and 2)", redacted, version)
	}

	// Another entity is untouched.
	var other string
	if err := app.QueryRow(ctx, `SELECT after::text FROM audit_log WHERE entity_id = $1::uuid`, otherUUID).Scan(&other); err != nil || !strings.Contains(other, "someone-else@example.se") {
		t.Errorf("another entity's row changed: %s (%v)", other, err)
	}
}

func TestINS05_RedactNeedsFieldsAndIsNotCallableByTheOwnerDirectly(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	if err := app.QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY[]::text[])`, someUUID).Scan(new(int)); err == nil {
		t.Error("an empty field list must be rejected")
	}
	// session_user = current_user, so the guard trigger refuses the owner here too.
	err := connect(t, owner).QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY['email'])`, someUUID).Scan(new(int))
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Errorf("owner calling the function directly: err = %v, want the append-only refusal", err)
	}
}

// Defence in depth: a definer function that tries to change anything but the
// redactable columns is stopped by the trigger, even though it runs as the owner.
func TestINS05_DefinerContextCanOnlyChangeTheRedactableColumns(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	ownerConn := connect(t, owner)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	if _, err := ownerConn.Exec(ctx, `
		CREATE FUNCTION tamper() RETURNS void LANGUAGE sql SECURITY DEFINER AS $$ UPDATE audit_log SET action = 'tampered' $$;
		GRANT EXECUTE ON FUNCTION tamper() TO linochi_erp_app`); err != nil {
		t.Fatal(err)
	}
	_, err := app.Exec(ctx, `SELECT tamper()`)
	if err == nil || !strings.Contains(err.Error(), "only") {
		t.Errorf("tampering with a locked column through a definer function: err = %v, want the column refusal", err)
	}
}

func TestINS05_RedactFunctionIsHardened(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))

	var definer bool
	var config []string
	if err := conn.QueryRow(ctx, `SELECT prosecdef, coalesce(proconfig, '{}') FROM pg_proc WHERE proname = 'audit_redact_personal_data'`).Scan(&definer, &config); err != nil {
		t.Fatal(err)
	}
	if !definer {
		t.Error("the function must be SECURITY DEFINER")
	}
	var pinned bool
	for _, c := range config {
		if strings.HasPrefix(c, "search_path=") {
			pinned = true
		}
	}
	if !pinned {
		t.Errorf("a SECURITY DEFINER function must pin search_path, config = %v", config)
	}

	var appMay, publicMay bool
	if err := conn.QueryRow(ctx, `
		SELECT has_function_privilege('linochi_erp_app', 'audit_redact_personal_data(text, uuid, text[])', 'EXECUTE'),
		       has_function_privilege('public', 'audit_redact_personal_data(text, uuid, text[])', 'EXECUTE')`).Scan(&appMay, &publicMay); err != nil {
		t.Fatal(err)
	}
	if !appMay || publicMay {
		t.Errorf("EXECUTE: app=%v public=%v, want true and false", appMay, publicMay)
	}
}

func TestINS05_AuditLogHasIndexesForTheFilters(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))
	rows, err := conn.Query(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'audit_log'`)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(defs, "\n")
	for _, want := range []string{"(entity_type, entity_id, created_at DESC)", "(actor_id, created_at DESC)", "(module, created_at DESC)", "(created_at DESC)"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing index on %s", want)
		}
	}
}
