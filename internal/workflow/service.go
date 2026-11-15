package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for the workflow engine (WF-01 to WF-07).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── Steps ────────────────────────────────────────────────────────────────────

// ListWorkflowSteps returns all active workflow steps (WF-01).
func (s *Service) ListWorkflowSteps(ctx context.Context) ([]Step, error) {
	return s.store.ListWorkflowSteps(ctx)
}

// CreateWorkflowStep creates a new workflow step (WF-01).
func (s *Service) CreateWorkflowStep(ctx context.Context, key, name, nameEn string, sortOrder int, isStart, isEnd bool) (Step, error) {
	var step Step
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		step, err = s.store.CreateWorkflowStep(ctx, id, key, name, nameEn, sortOrder, isStart, isEnd)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "workflow",
			EntityType: "step",
			EntityID:   step.ID,
			Action:     "Create",
			After:      map[string]any{"key": key, "name": name},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return step, err
}

// GetWorkflowStep returns a workflow step by ID.
func (s *Service) GetWorkflowStep(ctx context.Context, id uuid.UUID) (Step, error) {
	step, err := s.store.GetWorkflowStep(ctx, id)
	if err != nil {
		return step, ProblemError(err)
	}
	return step, nil
}

// UpdateWorkflowStep updates a workflow step (WF-01).
func (s *Service) UpdateWorkflowStep(ctx context.Context, name, nameEn *string, sortOrder *int, isStart, isEnd *bool, id uuid.UUID, version int) (Step, error) {
	step, err := s.store.UpdateWorkflowStep(ctx, name, nameEn, sortOrder, isStart, isEnd, id, version)
	if err != nil {
		return step, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "step",
		EntityID:   step.ID,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return step, nil
}

// ArchiveWorkflowStep archives a workflow step (NFK-09).
func (s *Service) ArchiveWorkflowStep(ctx context.Context, id uuid.UUID, version int) (Step, error) {
	step, err := s.store.ArchiveWorkflowStep(ctx, id, version)
	if err != nil {
		return step, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "step",
		EntityID:   step.ID,
		Action:     "Archive",
		Before:     map[string]any{"name": step.Name},
		Reason:     actor.ID.String(),
	})
	return step, nil
}

// ─── Transitions ──────────────────────────────────────────────────────────────

// ListWorkflowTransitions returns all workflow transitions (WF-02).
func (s *Service) ListWorkflowTransitions(ctx context.Context) ([]Transition, error) {
	return s.store.ListWorkflowTransitions(ctx)
}

// GetWorkflowTransition returns a transition by ID.
func (s *Service) GetWorkflowTransition(ctx context.Context, id uuid.UUID) (Transition, error) {
	transition, err := s.store.GetWorkflowTransition(ctx, id)
	if err != nil {
		return transition, ProblemError(err)
	}
	return transition, nil
}

// CreateWorkflowTransition creates a new transition (WF-01).
func (s *Service) CreateWorkflowTransition(ctx context.Context, stepKey, fromKey, toKey uuid.UUID, label, labelEn string, sortOrder int) (Transition, error) {
	var transition Transition
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		transition, err = s.store.CreateWorkflowTransition(ctx, id, stepKey, fromKey, toKey, label, labelEn, sortOrder)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "workflow",
			EntityType: "transition",
			EntityID:   transition.ID,
			Action:     "Create",
			After:      map[string]any{"from_key": fromKey.String(), "to_key": toKey.String()},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return transition, err
}

// UpdateWorkflowTransition updates a transition (WF-01).
func (s *Service) UpdateWorkflowTransition(ctx context.Context, fromKey, toKey *string, label, labelEn *string, sortOrder *int, id uuid.UUID, version int) (Transition, error) {
	transition, err := s.store.UpdateWorkflowTransition(ctx, fromKey, toKey, label, labelEn, sortOrder, id, version)
	if err != nil {
		return transition, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "transition",
		EntityID:   transition.ID,
		Action:     "Update",
		After:      map[string]any{"label": label},
		Reason:     actor.ID.String(),
	})
	return transition, nil
}

// ArchiveWorkflowTransition archives a transition (NFK-09).
func (s *Service) ArchiveWorkflowTransition(ctx context.Context, id uuid.UUID, version int) (Transition, error) {
	transition, err := s.store.ArchiveWorkflowTransition(ctx, id, version)
	if err != nil {
		return transition, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "transition",
		EntityID:   transition.ID,
		Action:     "Archive",
		Before:     map[string]any{"label": transition.Label},
		Reason:     actor.ID.String(),
	})
	return transition, nil
}

// ─── Conditions ───────────────────────────────────────────────────────────────

// ListWorkflowConditions returns all active conditions for a transition (WF-03).
func (s *Service) ListWorkflowConditions(ctx context.Context, transitionID uuid.UUID) ([]Condition, error) {
	return s.store.ListWorkflowConditions(ctx, transitionID)
}

// CreateWorkflowCondition creates a new condition (WF-03).
func (s *Service) CreateWorkflowCondition(ctx context.Context, transitionID uuid.UUID, kind string, config json.RawMessage) (Condition, error) {
	var condition Condition
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		condition, err = s.store.CreateWorkflowCondition(ctx, id, transitionID, kind, config)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "workflow",
			EntityType: "condition",
			EntityID:   condition.ID,
			Action:     "Create",
			After:      map[string]any{"kind": kind},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return condition, err
}

// ArchiveWorkflowCondition archives a condition (NFK-09).
func (s *Service) ArchiveWorkflowCondition(ctx context.Context, id uuid.UUID, version int) (Condition, error) {
	condition, err := s.store.ArchiveWorkflowCondition(ctx, id, version)
	if err != nil {
		return condition, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "condition",
		EntityID:   condition.ID,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return condition, nil
}

// ─── Effects ──────────────────────────────────────────────────────────────────

// ListWorkflowEffects returns all active effects for a transition (WF-04).
func (s *Service) ListWorkflowEffects(ctx context.Context, transitionID uuid.UUID) ([]Effect, error) {
	return s.store.ListWorkflowEffects(ctx, transitionID)
}

// CreateWorkflowEffect creates a new effect (WF-04).
func (s *Service) CreateWorkflowEffect(ctx context.Context, transitionID uuid.UUID, kind string, config json.RawMessage) (Effect, error) {
	var effect Effect
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		effect, err = s.store.CreateWorkflowEffect(ctx, id, transitionID, kind, config)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "workflow",
			EntityType: "effect",
			EntityID:   effect.ID,
			Action:     "Create",
			After:      map[string]any{"kind": kind},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return effect, err
}

// ArchiveWorkflowEffect archives an effect (NFK-09).
func (s *Service) ArchiveWorkflowEffect(ctx context.Context, id uuid.UUID, version int) (Effect, error) {
	effect, err := s.store.ArchiveWorkflowEffect(ctx, id, version)
	if err != nil {
		return effect, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "workflow",
		EntityType: "effect",
		EntityID:   effect.ID,
		Action:     "Archive",
		Reason:     actor.ID.String(),
	})
	return effect, nil
}

// ExecuteTransition runs all effects for a transition (WF-06).
func (s *Service) ExecuteTransition(ctx context.Context, transitionID, orderID uuid.UUID, effects []Effect) error {
	for _, eff := range effects {
		switch eff.Kind {
		case "reserve_stock":
			// Will be implemented in orders module
		case "release_stock":
			// Will be implemented in orders module
		case "withdraw_stock":
			// Will be implemented in orders module
		default:
			return fmt.Errorf("unknown effect kind: %s", eff.Kind)
		}
	}
	return nil
}
