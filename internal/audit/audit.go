// Package audit writes the change log (INS-05).
//
// Every write that changes data records an entry in the same transaction, so a
// change and its log row are saved together or not at all. Record takes a
// *db.Tx and not a pool, so it cannot be called outside a transaction.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// The actions with their own rules for what is stored. Any other action
// ("status_change", "login", …) stores only the fields that changed.
const (
	// Create stores the whole record as after.
	Create = "create"
	// Update stores the changed fields as before and after.
	Update = "update"
	// Archive stores the whole record as before. Archived records are hidden,
	// never deleted (NFK-09).
	Archive = "archive"
	// Redact is the entry that RedactPersonalData writes about itself.
	Redact = "redact"
)

// Entry describes one change.
type Entry struct {
	Module     string
	EntityType string
	EntityID   uuid.UUID
	// Action is Create, Update, Archive or any other verb for the change.
	Action string
	// Before and After are the record before and after, as structs or maps.
	// Create needs After and Archive needs Before. For anything else Record
	// stores only the fields that differ, and writes nothing if none do.
	Before, After any
	// Reason is optional, for example the reason of a workflow override (WF-05).
	Reason string
}

// Recorder writes audit entries and reports them to Loki and Prometheus.
type Recorder struct {
	logger  *slog.Logger
	entries *prometheus.CounterVec
}

// NewRecorder creates a Recorder and registers linochi_audit_entries_total.
func NewRecorder(logger *slog.Logger, metrics *obs.Metrics) (*Recorder, error) {
	entries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "linochi_audit_entries_total",
		Help: "Audit log entries written, by module and action.",
	}, []string{"module", "action"})
	if err := metrics.Registry.Register(entries); err != nil {
		return nil, fmt.Errorf("register audit metric: %w", err)
	}
	return &Recorder{logger: logger, entries: entries}, nil
}

func (e Entry) validate() error {
	switch {
	case e.Module == "":
		return fmt.Errorf("audit: entry has no module")
	case e.EntityType == "":
		return fmt.Errorf("audit: entry has no entity type")
	case e.EntityID == uuid.Nil:
		return fmt.Errorf("audit: entry has no entity id")
	case e.Action == "":
		return fmt.Errorf("audit: entry has no action")
	case e.Action == Create && e.After == nil:
		return fmt.Errorf("audit: a create entry needs the whole record as After")
	case e.Action == Archive && e.Before == nil:
		return fmt.Errorf("audit: an archive entry needs the whole record as Before")
	}
	return nil
}

// prepare works out what is stored. skip is true when there is nothing to record.
func (e Entry) prepare() (before, after map[string]any, names []string, skip bool, err error) {
	switch e.Action {
	case Create:
		after, err = toMap(e.After)
		if err != nil || len(after) == 0 {
			return nil, nil, nil, false, orEmpty(err, "audit: a create entry needs a record with fields")
		}
		return nil, after, keys(after), false, nil
	case Archive:
		before, err = toMap(e.Before)
		if err != nil || len(before) == 0 {
			return nil, nil, nil, false, orEmpty(err, "audit: an archive entry needs a record with fields")
		}
		return before, nil, keys(before), false, nil
	default:
		bm, err := toMap(e.Before)
		if err != nil {
			return nil, nil, nil, false, err
		}
		am, err := toMap(e.After)
		if err != nil {
			return nil, nil, nil, false, err
		}
		before, after, names = changedFields(bm, am, ignoredFields)
		return before, after, names, len(names) == 0, nil
	}
}

func orEmpty(err error, msg string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", msg)
}

// Record writes the entry in tx. The actor comes from the context, and without
// one the call fails with ErrNoActor, which rolls the transaction back. The log
// line and the metric are only produced after the commit, so a change that is
// rolled back never shows up in Loki. The line holds the names of the changed
// fields and never their values.
func (r *Recorder) Record(ctx context.Context, tx *db.Tx, e Entry) error {
	if err := e.validate(); err != nil {
		return err
	}
	actor, ok := ActorFrom(ctx)
	if !ok {
		return ErrNoActor
	}
	before, after, names, skip, err := e.prepare()
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	beforeJSON, err := marshalOrNil(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalOrNil(after)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("audit: new id: %w", err)
	}

	params := dbgen.InsertAuditEntryParams{
		ID: id, ActorType: string(actor.Type), ActorLabel: actor.Label,
		Module: e.Module, EntityType: e.EntityType, EntityID: e.EntityID, Action: e.Action,
		Before: beforeJSON, After: afterJSON,
	}
	if actor.Type != ActorSystem {
		actorID := actor.ID
		params.ActorID = &actorID
	}
	if e.Reason != "" {
		reason := e.Reason
		params.Reason = &reason
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		traceID := sc.TraceID().String()
		params.TraceID = &traceID
	}
	if _, err := tx.Queries().InsertAuditEntry(ctx, params); err != nil {
		return fmt.Errorf("audit: insert entry: %w", err)
	}

	tx.AfterCommit(func() { r.published(ctx, id, actor, e, names) })
	return nil
}

// published runs after the commit: the log line for Loki and the metric.
func (r *Recorder) published(ctx context.Context, id uuid.UUID, actor Actor, e Entry, fields []string) {
	attrs := []slog.Attr{
		slog.String("audit_id", id.String()),
		slog.String("actor_type", string(actor.Type)),
		slog.String("module", e.Module),
		slog.String("entity_type", e.EntityType),
		slog.String("entity_id", e.EntityID.String()),
		slog.String("action", e.Action),
		slog.Any("fields", fields), // names only, never values
	}
	if actor.Type != ActorSystem {
		attrs = append(attrs, slog.String("actor_id", actor.ID.String()))
	}
	if actor.Label != "" {
		attrs = append(attrs, slog.String("actor_label", actor.Label))
	}
	// The request may be over by now; the values in ctx (the trace) are kept.
	r.logger.LogAttrs(context.WithoutCancel(ctx), slog.LevelInfo, "audit", attrs...)
	r.entries.WithLabelValues(e.Module, e.Action).Inc()
}

// RedactPersonalData is the GDPR step of KUN-07: it replaces the old values of
// the given fields of one record in the audit log with "[redacted]" and returns
// how many rows changed. It calls the SECURITY DEFINER function in the database,
// which is the only way to change old audit rows, and then records an entry
// about the redaction itself: which fields and how many rows, no values.
func (r *Recorder) RedactPersonalData(ctx context.Context, tx *db.Tx, module, entityType string, entityID uuid.UUID, fields []string) (int, error) {
	if len(fields) == 0 {
		return 0, fmt.Errorf("audit: redact needs at least one field")
	}
	probe := Entry{Module: module, EntityType: entityType, EntityID: entityID, Action: Redact}
	if err := probe.validate(); err != nil {
		return 0, err
	}
	if _, ok := ActorFrom(ctx); !ok {
		return 0, ErrNoActor
	}
	n, err := tx.Queries().RedactAuditPersonalData(ctx, dbgen.RedactAuditPersonalDataParams{
		EntityType: entityType, EntityID: entityID, Fields: fields,
	})
	if err != nil {
		return 0, fmt.Errorf("audit: redact personal data: %w", err)
	}
	probe.After = map[string]any{"fields": fields, "rows": n}
	if err := r.Record(ctx, tx, probe); err != nil {
		return 0, err
	}
	return int(n), nil
}

func marshalOrNil(m map[string]any) (json.RawMessage, error) {
	if len(m) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("audit: encode: %w", err)
	}
	return raw, nil
}
