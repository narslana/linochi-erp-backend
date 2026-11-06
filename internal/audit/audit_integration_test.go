//go:build integration

package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

var server *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		server = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}

// widget is a made-up record: the audit package works on any struct or map.
type widget struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Email   string    `json:"email"`
	Color   *string   `json:"color"`
	Version int32     `json:"version"`
	Updated time.Time `json:"updated_at"`
}

type env struct {
	pool    *pgxpool.Pool
	rec     *audit.Recorder
	logs    *bytes.Buffer
	metrics *obs.Metrics
	tp      *sdktrace.TracerProvider
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := server.NewMigratedDatabase(t)
	e := &env{logs: &bytes.Buffer{}, metrics: obs.NewMetrics("test", "abc1234")}
	e.tp = sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = e.tp.Shutdown(context.Background()) })

	pool, err := db.Open(context.Background(), db.Options{
		URL: testdb.AppURL(dsn), MaxConns: 3, Metrics: e.metrics, Tracer: e.tp,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e.pool = pool

	e.rec, err = audit.NewRecorder(obs.NewLogger(e.logs, slog.LevelDebug), e.metrics)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

var (
	userID   = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000a1")
	widgetID = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000b1")
)

func actorCtx() context.Context {
	return audit.WithActor(context.Background(), audit.UserActor(userID))
}

// record runs one entry in its own transaction.
func (e *env) record(ctx context.Context, entry audit.Entry) error {
	return db.InTx(ctx, e.pool, func(tx *db.Tx) error { return e.rec.Record(ctx, tx, entry) })
}

func (e *env) entries(t *testing.T) []dbgen.AuditLog {
	t.Helper()
	rows, err := dbgen.New(e.pool).ListAuditEntriesForEntity(context.Background(),
		dbgen.ListAuditEntriesForEntityParams{EntityType: "widget", EntityID: widgetID})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func asMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if raw == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("not a JSON object: %s", raw)
	}
	return m
}

func wid(name, email string, version int32) widget {
	return widget{ID: widgetID, Name: name, Email: email, Version: version, Updated: time.Now()}
}

func entry(action string, before, after any) audit.Entry {
	return audit.Entry{Module: "widgets", EntityType: "widget", EntityID: widgetID, Action: action, Before: before, After: after}
}

func TestINS05_RecordWritesInTheSameTransaction(t *testing.T) {
	e := newEnv(t)
	ctx := actorCtx()

	boom := errors.New("boom")
	err := db.InTx(ctx, e.pool, func(tx *db.Tx) error {
		if err := e.rec.Record(ctx, tx, entry(audit.Create, nil, wid("A", "a@example.se", 1))); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(e.entries(t)); n != 0 {
		t.Fatalf("a rolled-back change left %d audit rows", n)
	}

	if err := e.record(ctx, entry(audit.Create, nil, wid("A", "a@example.se", 1))); err != nil {
		t.Fatal(err)
	}
	if n := len(e.entries(t)); n != 1 {
		t.Fatalf("a committed change left %d audit rows, want 1", n)
	}
}

func TestINS05_CreateStoresTheWholeRecordAsAfter(t *testing.T) {
	e := newEnv(t)
	if err := e.record(actorCtx(), entry(audit.Create, nil, wid("A", "a@example.se", 1))); err != nil {
		t.Fatal(err)
	}
	row := e.entries(t)[0]
	if row.Before != nil {
		t.Errorf("before = %s, want NULL for a create", row.Before)
	}
	after := asMap(t, row.After)
	for _, k := range []string{"id", "name", "email", "color", "version", "updated_at"} {
		if _, ok := after[k]; !ok {
			t.Errorf("the whole record belongs in after, but %q is missing: %v", k, after)
		}
	}
	if row.Action != audit.Create || row.Module != "widgets" || row.EntityType != "widget" {
		t.Errorf("row = %+v", row)
	}
}

func TestINS05_UpdateStoresOnlyChangedFieldsWithBeforeAndAfter(t *testing.T) {
	e := newEnv(t)
	old := wid("Anna", "a@example.se", 1)
	now := wid("Anna", "anna@example.se", 2)
	if err := e.record(actorCtx(), entry(audit.Update, old, now)); err != nil {
		t.Fatal(err)
	}
	row := e.entries(t)[0]
	before, after := asMap(t, row.Before), asMap(t, row.After)
	if len(before) != 1 || before["email"] != "a@example.se" {
		t.Errorf("before = %v, want only the old email", before)
	}
	if len(after) != 1 || after["email"] != "anna@example.se" {
		t.Errorf("after = %v, want only the new email (not name, and not version or updated_at)", after)
	}
}

func TestINS05_UpdateWithoutRealChangesWritesNothing(t *testing.T) {
	e := newEnv(t)
	// Only the bookkeeping fields differ.
	if err := e.record(actorCtx(), entry(audit.Update, wid("A", "a@example.se", 1), wid("A", "a@example.se", 2))); err != nil {
		t.Fatal(err)
	}
	if n := len(e.entries(t)); n != 0 {
		t.Errorf("an update without changes wrote %d rows", n)
	}
	if got := e.count(t, "widgets", "update"); got != 0 {
		t.Errorf("an update without changes was counted: %v", got)
	}
	if strings.Contains(e.logs.String(), `"msg":"audit"`) {
		t.Errorf("an update without changes was logged: %s", e.logs.String())
	}
}

func TestINS05_ArchiveStoresTheWholeRecordAsBefore(t *testing.T) {
	e := newEnv(t)
	if err := e.record(actorCtx(), entry(audit.Archive, wid("A", "a@example.se", 3), nil)); err != nil {
		t.Fatal(err)
	}
	row := e.entries(t)[0]
	if row.After != nil {
		t.Errorf("after = %s, want NULL for an archive", row.After)
	}
	before := asMap(t, row.Before)
	for _, k := range []string{"id", "name", "email", "color", "version", "updated_at"} {
		if _, ok := before[k]; !ok {
			t.Errorf("the whole record belongs in before, but %q is missing: %v", k, before)
		}
	}
}

// An unattributed change must not be saved: the whole transaction is rolled back.
func TestINS05_MissingActorRollsBackTheChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background() // no actor

	err := db.InTx(ctx, e.pool, func(tx *db.Tx) error {
		if _, err := tx.Queries().InsertUserIfNew(ctx, dbgen.InsertUserIfNewParams{
			ID: uuid.Must(uuid.NewV7()), OidcIssuer: "i", OidcSubject: "s", Email: "x@example.se"}); err != nil {
			return err
		}
		return e.rec.Record(ctx, tx, entry(audit.Create, nil, wid("A", "a@example.se", 1)))
	})
	if !errors.Is(err, audit.ErrNoActor) {
		t.Fatalf("err = %v, want audit.ErrNoActor", err)
	}
	var users int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 0 {
		t.Errorf("the unattributed change was saved: users=%d err=%v", users, err)
	}
}

func TestINS05_ActorsAreRecorded(t *testing.T) {
	e := newEnv(t)
	keyID := uuid.MustParse("0194fa5e-0000-7000-8000-0000000000c1")
	for _, a := range []audit.Actor{audit.UserActor(userID), audit.APIKeyActor(keyID), audit.SystemActor("oidc-login")} {
		if err := e.record(audit.WithActor(context.Background(), a), entry(audit.Create, nil, wid("A", "a@example.se", 1))); err != nil {
			t.Fatalf("%s: %v", a.Type, err)
		}
	}
	rows := e.entries(t)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].ActorType != "user" || *rows[0].ActorID != userID || rows[0].ActorLabel != "" {
		t.Errorf("user actor: %+v", rows[0])
	}
	if rows[1].ActorType != "api_key" || *rows[1].ActorID != keyID {
		t.Errorf("api key actor: %+v", rows[1])
	}
	if rows[2].ActorType != "system" || rows[2].ActorID != nil || rows[2].ActorLabel != "oidc-login" {
		t.Errorf("system actor: %+v", rows[2])
	}
}

func TestINS05_TraceIDAndReasonAreStored(t *testing.T) {
	e := newEnv(t)
	ctx, span := e.tp.Tracer("test").Start(actorCtx(), "request")
	defer span.End()

	en := entry(audit.Update, wid("A", "a@example.se", 1), wid("A", "b@example.se", 2))
	en.Reason = "customer asked"
	if err := e.record(ctx, en); err != nil {
		t.Fatal(err)
	}
	if err := e.record(actorCtx(), entry(audit.Update, wid("A", "b@example.se", 2), wid("A", "c@example.se", 3))); err != nil {
		t.Fatal(err)
	}
	rows := e.entries(t)
	if rows[0].TraceID == nil || *rows[0].TraceID != span.SpanContext().TraceID().String() {
		t.Errorf("trace_id = %v, want %s", rows[0].TraceID, span.SpanContext().TraceID())
	}
	if rows[0].Reason == nil || *rows[0].Reason != "customer asked" {
		t.Errorf("reason = %v", rows[0].Reason)
	}
	if rows[1].TraceID != nil || rows[1].Reason != nil {
		t.Errorf("a row without a span or reason must have NULL for both: %+v", rows[1])
	}
}

func TestINS05_EntryIsValidated(t *testing.T) {
	e := newEnv(t)
	full := entry(audit.Create, nil, wid("A", "a@example.se", 1))
	tests := []struct {
		name   string
		mutate func(*audit.Entry)
	}{
		{"no module", func(en *audit.Entry) { en.Module = "" }},
		{"no entity type", func(en *audit.Entry) { en.EntityType = "" }},
		{"no entity id", func(en *audit.Entry) { en.EntityID = uuid.Nil }},
		{"no action", func(en *audit.Entry) { en.Action = "" }},
		{"create without after", func(en *audit.Entry) { en.After = nil }},
		{"archive without before", func(en *audit.Entry) { en.Action = audit.Archive; en.Before = nil }},
		{"a record that is not an object", func(en *audit.Entry) { en.After = "just text" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			en := full
			tt.mutate(&en)
			err := e.record(actorCtx(), en)
			if err == nil || errors.Is(err, audit.ErrNoActor) {
				t.Fatalf("err = %v, want a validation error", err)
			}
		})
	}
	if n := len(e.entries(t)); n != 0 {
		t.Errorf("invalid entries wrote %d rows", n)
	}
}

// GDPR (KUN-07): old values of personal fields are replaced, and the redaction
// is itself in the log, with the names of the fields and no values.
func TestINS05_RedactPersonalDataReplacesOldValuesAndIsItselfAudited(t *testing.T) {
	e := newEnv(t)
	ctx := actorCtx()
	if err := e.record(ctx, entry(audit.Create, nil, wid("Anna Svensson", "anna@example.se", 1))); err != nil {
		t.Fatal(err)
	}
	if err := e.record(ctx, entry(audit.Update, wid("Anna Svensson", "anna@example.se", 1), wid("Anna Svensson", "anna.s@example.se", 2))); err != nil {
		t.Fatal(err)
	}

	var n int
	err := db.InTx(ctx, e.pool, func(tx *db.Tx) (err error) {
		n, err = e.rec.RedactPersonalData(ctx, tx, "widgets", "widget", widgetID, []string{"name", "email"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("rows redacted = %d, want 2", n)
	}

	rows := e.entries(t)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want the two old ones and one about the redaction", len(rows))
	}
	for _, r := range rows[:2] {
		all := string(r.Before) + string(r.After)
		for _, pii := range []string{"Anna", "anna@example.se", "anna.s@example.se"} {
			if strings.Contains(all, pii) {
				t.Errorf("%q survived in row %s: %s", pii, r.Action, all)
			}
		}
		if r.RedactedAt == nil {
			t.Errorf("row %s has no redacted_at", r.Action)
		}
	}
	last := rows[2]
	if last.Action != audit.Redact || last.RedactedAt != nil {
		t.Errorf("the redaction row: %+v", last)
	}
	after := asMap(t, last.After)
	if after["rows"] != float64(2) || len(after["fields"].([]any)) != 2 {
		t.Errorf("the redaction row must say which fields and how many rows: %v", after)
	}
	if strings.Contains(string(last.After), "anna") || strings.Contains(string(last.After), "Anna") {
		t.Errorf("the redaction row holds values: %s", last.After)
	}
}

func TestINS05_RedactNeedsAnActorAndFields(t *testing.T) {
	e := newEnv(t)
	err := db.InTx(context.Background(), e.pool, func(tx *db.Tx) error {
		_, err := e.rec.RedactPersonalData(context.Background(), tx, "widgets", "widget", widgetID, []string{"email"})
		return err
	})
	if !errors.Is(err, audit.ErrNoActor) {
		t.Errorf("no actor: err = %v, want audit.ErrNoActor", err)
	}
	err = db.InTx(actorCtx(), e.pool, func(tx *db.Tx) error {
		_, err := e.rec.RedactPersonalData(actorCtx(), tx, "widgets", "widget", widgetID, nil)
		return err
	})
	if err == nil {
		t.Error("an empty field list must be rejected")
	}
}

// What goes to Loki: that something changed and which fields, never the values,
// so personal data cannot reach Loki whatever a field is called. And only after
// the commit, so a change that was rolled back never shows up.
func TestOBS_AuditLogLineAfterCommitHasFieldNamesNotValues(t *testing.T) {
	e := newEnv(t)
	ctx, span := e.tp.Tracer("test").Start(actorCtx(), "request")
	defer span.End()

	boom := errors.New("boom")
	_ = db.InTx(ctx, e.pool, func(tx *db.Tx) error {
		if err := e.rec.Record(ctx, tx, entry(audit.Update, wid("A", "secret-old@example.se", 1), wid("A", "secret-new@example.se", 2))); err != nil {
			return err
		}
		if strings.Contains(e.logs.String(), `"msg":"audit"`) {
			t.Error("the audit line was logged before the commit")
		}
		return boom
	})
	if strings.Contains(e.logs.String(), `"msg":"audit"`) {
		t.Fatalf("a rolled-back change was logged: %s", e.logs.String())
	}

	if err := e.record(ctx, entry(audit.Update, wid("A", "secret-old@example.se", 1), wid("A", "secret-new@example.se", 2))); err != nil {
		t.Fatal(err)
	}
	out := e.logs.String()
	if strings.Contains(out, "secret-old") || strings.Contains(out, "secret-new") {
		t.Fatalf("values leaked into the log: %s", out)
	}
	var line map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["msg"] == "audit" {
			line = m
		}
	}
	if line == nil {
		t.Fatalf("no audit line in %s", out)
	}
	fields, _ := line["fields"].([]any)
	if len(fields) != 1 || fields[0] != "email" {
		t.Errorf("fields = %v, want [email]", line["fields"])
	}
	for k, want := range map[string]any{
		"module": "widgets", "entity_type": "widget", "entity_id": widgetID.String(),
		"action": "update", "actor_type": "user", "actor_id": userID.String(),
		"trace_id": span.SpanContext().TraceID().String(),
	} {
		if line[k] != want {
			t.Errorf("%s = %v, want %v", k, line[k], want)
		}
	}
	if line["audit_id"] == nil || line["audit_id"] == "" {
		t.Error("audit_id is missing")
	}
}

func TestOBS_AuditMetricCountsPerModuleAndActionAfterCommit(t *testing.T) {
	e := newEnv(t)
	ctx := actorCtx()

	_ = db.InTx(ctx, e.pool, func(tx *db.Tx) error {
		_ = e.rec.Record(ctx, tx, entry(audit.Create, nil, wid("A", "a@example.se", 1)))
		return errors.New("rolled back")
	})
	if got := e.count(t, "widgets", "create"); got != 0 {
		t.Fatalf("a rolled-back entry was counted: %v", got)
	}
	for range 2 {
		if err := e.record(ctx, entry(audit.Create, nil, wid("A", "a@example.se", 1))); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.record(ctx, entry(audit.Archive, wid("A", "a@example.se", 1), nil)); err != nil {
		t.Fatal(err)
	}
	if got := e.count(t, "widgets", "create"); got != 2 {
		t.Errorf("create counted %v, want 2", got)
	}
	if got := e.count(t, "widgets", "archive"); got != 1 {
		t.Errorf("archive counted %v, want 1", got)
	}
}

func (e *env) count(t *testing.T, module, action string) float64 {
	t.Helper()
	families, err := e.metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "linochi_audit_entries_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			l := map[string]string{}
			for _, p := range m.GetLabel() {
				l[p.GetName()] = p.GetValue()
			}
			if l["module"] == module && l["action"] == action {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}
