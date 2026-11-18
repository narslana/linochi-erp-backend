package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// orderLineRef is the reference type stock movements of an order line carry.
const orderLineRef = "order_line"

// Service is the workflow engine. Every status change of an order goes through
// Transition (WF-02, WF-05); stock effects run in the same transaction (WF-03).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
	inv   *inventory.Service
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool, inv *inventory.Service) *Service {
	return &Service{store: store, rec: rec, pool: pool, inv: inv}
}

// ─── Versions (WF-01, WF-06, WF-07) ──────────────────────────────────────────

// ActiveVersion is the version new orders start in.
func (s *Service) ActiveVersion(ctx context.Context) (Version, error) {
	row, err := s.store.q.GetActiveWorkflowVersion(ctx)
	if err != nil {
		return Version{}, notFoundOr(err, "workflow version")
	}
	return s.loadVersion(ctx, s.store.q, row)
}

// GetVersion returns one version with its steps and transitions.
func (s *Service) GetVersion(ctx context.Context, id uuid.UUID) (Version, error) {
	row, err := s.store.q.GetWorkflowVersion(ctx, id)
	if err != nil {
		return Version{}, notFoundOr(err, "workflow version")
	}
	return s.loadVersion(ctx, s.store.q, row)
}

// ListVersions returns all versions, newest first.
func (s *Service) ListVersions(ctx context.Context) ([]Version, error) {
	rows, err := s.store.q.ListWorkflowVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflow versions: %w", err)
	}
	out := make([]Version, 0, len(rows))
	for _, r := range rows {
		v, err := s.loadVersion(ctx, s.store.q, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) loadVersion(ctx context.Context, q *dbgen.Queries, row dbgen.WorkflowVersion) (Version, error) {
	steps, err := q.ListWorkflowSteps(ctx, row.ID)
	if err != nil {
		return Version{}, fmt.Errorf("list workflow steps: %w", err)
	}
	trs, err := q.ListWorkflowTransitions(ctx, row.ID)
	if err != nil {
		return Version{}, fmt.Errorf("list workflow transitions: %w", err)
	}
	v := Version{ID: row.ID, WorkflowID: row.WorkflowID, Version: int(row.Version), PublishedAt: row.PublishedAt, IsActive: row.IsActive}
	for _, st := range steps {
		v.Steps = append(v.Steps, stepDef(st))
	}
	for _, tr := range trs {
		d := TransitionDef{From: tr.FromKey, To: tr.ToKey}
		if tr.Effect != nil {
			d.Effect = *tr.Effect
		}
		v.Transitions = append(v.Transitions, d)
	}
	return v, nil
}

func stepDef(st dbgen.WorkflowStep) StepDef {
	return StepDef{Key: st.Key, SortOrder: int(st.SortOrder), IsStart: st.IsStart, IsFinal: st.IsFinal, IsLocked: st.IsLocked, StockPhase: st.StockPhase}
}

// PublishVersion validates a flow (WF-06) and publishes it as the next version
// of the default flow, active from now on (WF-07). Orders in progress keep the
// version they started in. A flow that breaks a rule is refused with 422 and
// nothing is saved.
func (s *Service) PublishVersion(ctx context.Context, def VersionDefinition) (Version, error) {
	if problems := Validate(def); len(problems) > 0 {
		return Version{}, &InvalidFlowError{Problems: problems}
	}
	var out Version
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		q := tx.Queries()
		wf, err := q.DefaultWorkflowForUpdate(ctx)
		if err != nil {
			return fmt.Errorf("lock default workflow: %w", err)
		}
		next, err := q.NextWorkflowVersionNumber(ctx, wf.ID)
		if err != nil {
			return fmt.Errorf("next version number: %w", err)
		}
		if err := q.DeactivateWorkflowVersions(ctx, wf.ID); err != nil {
			return fmt.Errorf("deactivate versions: %w", err)
		}
		ver, err := q.InsertWorkflowVersion(ctx, dbgen.InsertWorkflowVersionParams{WorkflowID: wf.ID, Version: next})
		if err != nil {
			return fmt.Errorf("insert version: %w", err)
		}
		ids := map[string]uuid.UUID{}
		for _, st := range def.Steps {
			row, err := q.InsertWorkflowStep(ctx, dbgen.InsertWorkflowStepParams{
				VersionID: ver.ID, Key: st.Key, SortOrder: int32(st.SortOrder), //nolint:gosec // small flow definition
				IsStart: st.IsStart, IsFinal: st.IsFinal, IsLocked: st.IsLocked, StockPhase: string(phaseOf(st)),
			})
			if err != nil {
				return fmt.Errorf("insert step %q: %w", st.Key, err)
			}
			ids[st.Key] = row.ID
		}
		for _, tr := range def.Transitions {
			var eff *string
			if tr.Effect != "" {
				eff = &tr.Effect
			}
			if err := q.InsertWorkflowTransition(ctx, dbgen.InsertWorkflowTransitionParams{
				VersionID: ver.ID, FromStepID: ids[tr.From], ToStepID: ids[tr.To], Effect: eff,
			}); err != nil {
				return fmt.Errorf("insert transition %q to %q: %w", tr.From, tr.To, err)
			}
		}
		out, err = s.loadVersion(ctx, q, ver)
		if err != nil {
			return err
		}
		return s.rec.Record(ctx, tx, audit.Entry{
			Module: "workflow", EntityType: "workflow_version", EntityID: ver.ID, Action: audit.Create,
			After: map[string]any{"version": next, "steps": len(def.Steps), "transitions": len(def.Transitions)},
		})
	})
	return out, err
}

// ─── History (WF-10) ─────────────────────────────────────────────────────────

// History returns the status changes of an order, oldest first.
func (s *Service) History(ctx context.Context, orderID uuid.UUID) ([]HistoryEntry, error) {
	rows, err := s.store.q.ListOrderStatusHistory(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list status history: %w", err)
	}
	out := make([]HistoryEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryEntry{ID: r.ID, OrderID: r.OrderID, FromKey: r.FromKey, ToKey: r.ToKey,
			ActorID: r.ActorID, Reason: r.Reason, IsOverride: r.IsOverride, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// TimeInSteps works out how long an order was in each step from its history
// (WF-10). The step it is in now counts until now. The first interval runs from
// the creation of the order to its first change. Intervals come in order, one
// per visit.
func (s *Service) TimeInSteps(ctx context.Context, orderID uuid.UUID, now time.Time) ([]StepTime, error) {
	hist, err := s.History(ctx, orderID)
	if err != nil {
		return nil, err
	}
	o, err := s.store.q.GetOrder(ctx, orderID)
	if err != nil {
		return nil, notFoundOr(err, "order")
	}
	var out []StepTime
	key, since := o.Status, o.CreatedAt
	if len(hist) > 0 {
		key = hist[0].FromKey
	}
	for _, h := range hist {
		out = append(out, StepTime{Key: key, Duration: max(0, h.CreatedAt.Sub(since))})
		key, since = h.ToKey, h.CreatedAt
	}
	return append(out, StepTime{Key: key, Duration: max(0, now.Sub(since))}), nil
}

// ─── The engine (WF-02, WF-03, WF-05) ────────────────────────────────────────

// ChangeStatus is Transition in a transaction of its own.
func (s *Service) ChangeStatus(ctx context.Context, orderID uuid.UUID, toKey string, actor Actor, reason string, override bool) (Result, error) {
	var res Result
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) (err error) {
		res, err = s.Transition(ctx, tx, orderID, toKey, actor, reason, override)
		return err
	})
	return res, err
}

// Preview returns the effects a status change would have, without running them
// (WF-05). It checks the move like ChangeStatus does; who may do it is checked
// when it is done, not here.
func (s *Service) Preview(ctx context.Context, orderID uuid.UUID, toKey string, override bool) ([]PlannedEffect, error) {
	o, err := s.store.q.GetOrderForTransition(ctx, orderID)
	if err != nil {
		return nil, notFoundOr(err, "order")
	}
	p, err := s.plan(ctx, s.store.q, o.Status, o.WorkflowVersionID, orderID, toKey, override)
	if err != nil {
		return nil, err
	}
	return p.effects, nil
}

// Transition changes the status of an order inside the caller's transaction: it
// checks the move, runs the stock effects, saves the new status with the order's
// real version, and writes the history and audit rows. If anything fails the
// caller's rollback undoes all of it (WF-03).
func (s *Service) Transition(ctx context.Context, tx *db.Tx, orderID uuid.UUID, toKey string, actor Actor, reason string, override bool) (Result, error) {
	if override {
		if !actor.Admin {
			return Result{}, &ForbiddenError{}
		}
		if strings.TrimSpace(reason) == "" {
			return Result{}, &ReasonRequiredError{}
		}
	}
	q := tx.Queries()
	o, err := q.LockOrderForTransition(ctx, orderID)
	if err != nil {
		return Result{}, notFoundOr(err, "order")
	}
	p, err := s.plan(ctx, q, o.Status, o.WorkflowVersionID, orderID, toKey, override)
	if err != nil {
		return Result{}, err
	}
	res := Result{OrderID: orderID, FromKey: p.from.Key, ToKey: p.to.Key, Effects: p.effects, Warnings: []string{}}

	if err := s.runEffects(ctx, tx, p, &res); err != nil {
		return Result{}, err
	}

	if _, err := q.UpdateOrderStatus(ctx, dbgen.UpdateOrderStatusParams{ID: orderID, Status: p.to.Key, Version: o.Version}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, db.ErrStaleVersion
		}
		return Result{}, fmt.Errorf("update order status: %w", err)
	}
	if err := q.InsertOrderStatusHistory(ctx, dbgen.InsertOrderStatusHistoryParams{
		OrderID: orderID, FromKey: p.from.Key, ToKey: p.to.Key, ActorID: actor.UserID,
		Reason: strings.TrimSpace(reason), IsOverride: override,
	}); err != nil {
		return Result{}, fmt.Errorf("insert status history: %w", err)
	}
	action := "status_change"
	if override {
		action = "status_override"
	}
	if err := s.rec.Record(ctx, tx, audit.Entry{
		Module: "orders", EntityType: "order", EntityID: orderID, Action: action,
		Before: map[string]any{"status": p.from.Key}, After: map[string]any{"status": p.to.Key},
		Reason: strings.TrimSpace(reason),
	}); err != nil {
		return Result{}, err
	}
	return res, nil
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", db.ErrNotFound, what)
	}
	return fmt.Errorf("%s: %w", what, err)
}
