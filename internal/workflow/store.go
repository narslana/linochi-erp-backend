package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the workflow module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Steps ────────────────────────────────────────────────────────────────────

func (s *Store) ListWorkflowSteps(ctx context.Context) ([]Step, error) {
	rows, err := s.q.ListWorkflowSteps(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Step, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToStep(r))
	}
	return out, nil
}

func (s *Store) GetWorkflowStep(ctx context.Context, key string) (Step, error) {
	row, err := s.q.GetWorkflowStep(ctx, key)
	if err != nil {
		return Step{}, err
	}
	return dbRowToStep(row), nil
}

func (s *Store) CreateWorkflowStep(ctx context.Context, id uuid.UUID, key, name, nameEn string, sortOrder int, isStart, isEnd bool) (Step, error) {
	row, err := s.q.CreateWorkflowStep(ctx, dbgen.CreateWorkflowStepParams{
		ID:        id,
		Key:       key,
		Name:      name,
		NameEn:    nameEn,
		SortOrder: int32(sortOrder), //nolint:gosec // sort order is bounded by business logic
		IsStart:   isStart,
		IsEnd:     isEnd,
	})
	if err != nil {
		return Step{}, err
	}
	return dbRowToStep(row), nil
}

func (s *Store) UpdateWorkflowStep(ctx context.Context, name, nameEn *string, sortOrder *int, isStart, isEnd *bool, key string, version int) (Step, error) {
	var params dbgen.UpdateWorkflowStepParams
	params.Key = key
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if name != nil {
		params.Name = *name
	}
	if nameEn != nil {
		params.NameEn = *nameEn
	}
	if sortOrder != nil {
		params.SortOrder = int32(*sortOrder) //nolint:gosec // sort order is bounded by business logic
	}
	if isStart != nil {
		params.IsStart = *isStart
	}
	if isEnd != nil {
		params.IsEnd = *isEnd
	}
	row, err := s.q.UpdateWorkflowStep(ctx, params)
	if err != nil {
		return Step{}, err
	}
	return dbRowToStep(row), nil
}

func (s *Store) ArchiveWorkflowStep(ctx context.Context, key string, version int) (Step, error) {
	row, err := s.q.ArchiveWorkflowStep(ctx, dbgen.ArchiveWorkflowStepParams{Key: key, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Step{}, err
	}
	return dbRowToStep(row), nil
}

// ─── Transitions ──────────────────────────────────────────────────────────────

func (s *Store) ListWorkflowTransitions(ctx context.Context, stepKey string) ([]Transition, error) {
	rows, err := s.q.ListWorkflowTransitions(ctx, stepKey)
	if err != nil {
		return nil, err
	}
	out := make([]Transition, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToTransition(r))
	}
	return out, nil
}

func (s *Store) GetWorkflowTransition(ctx context.Context, id uuid.UUID) (Transition, error) {
	row, err := s.q.GetWorkflowTransition(ctx, id)
	if err != nil {
		return Transition{}, err
	}
	return dbRowToTransition(row), nil
}

func (s *Store) CreateWorkflowTransition(ctx context.Context, id, stepKey, fromKey, toKey uuid.UUID, label, labelEn string, sortOrder int) (Transition, error) {
	row, err := s.q.CreateWorkflowTransition(ctx, dbgen.CreateWorkflowTransitionParams{
		ID:        id,
		StepKey:   stepKey.String(),
		FromKey:   fromKey.String(),
		ToKey:     toKey.String(),
		Label:     label,
		LabelEn:   labelEn,
		SortOrder: int32(sortOrder), //nolint:gosec // sort order is bounded by business logic
	})
	if err != nil {
		return Transition{}, err
	}
	return dbRowToTransition(row), nil
}

func (s *Store) UpdateWorkflowTransition(ctx context.Context, fromKey, toKey *string, label, labelEn *string, sortOrder *int, id uuid.UUID, version int) (Transition, error) {
	var params dbgen.UpdateWorkflowTransitionParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if fromKey != nil {
		params.FromKey = *fromKey
	}
	if toKey != nil {
		params.ToKey = *toKey
	}
	if label != nil {
		params.Label = *label
	}
	if labelEn != nil {
		params.LabelEn = *labelEn
	}
	if sortOrder != nil {
		params.SortOrder = int32(*sortOrder) //nolint:gosec // sort order is bounded by business logic
	}
	row, err := s.q.UpdateWorkflowTransition(ctx, params)
	if err != nil {
		return Transition{}, err
	}
	return dbRowToTransition(row), nil
}

func (s *Store) ArchiveWorkflowTransition(ctx context.Context, id uuid.UUID, version int) (Transition, error) {
	row, err := s.q.ArchiveWorkflowTransition(ctx, dbgen.ArchiveWorkflowTransitionParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Transition{}, err
	}
	return dbRowToTransition(row), nil
}

// ─── Conditions ───────────────────────────────────────────────────────────────

func (s *Store) ListWorkflowConditions(ctx context.Context, transitionID uuid.UUID) ([]Condition, error) {
	rows, err := s.q.ListWorkflowConditions(ctx, transitionID)
	if err != nil {
		return nil, err
	}
	out := make([]Condition, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToCondition(r))
	}
	return out, nil
}

func (s *Store) CreateWorkflowCondition(ctx context.Context, id, transitionID uuid.UUID, kind string, config json.RawMessage) (Condition, error) {
	row, err := s.q.CreateWorkflowCondition(ctx, dbgen.CreateWorkflowConditionParams{
		ID:           id,
		TransitionID: transitionID,
		Kind:         kind,
		Config:       config,
	})
	if err != nil {
		return Condition{}, err
	}
	return dbRowToCondition(row), nil
}

func (s *Store) ArchiveWorkflowCondition(ctx context.Context, id uuid.UUID, version int) (Condition, error) {
	row, err := s.q.ArchiveWorkflowCondition(ctx, dbgen.ArchiveWorkflowConditionParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Condition{}, err
	}
	return dbRowToCondition(row), nil
}

// ─── Effects ──────────────────────────────────────────────────────────────────

func (s *Store) ListWorkflowEffects(ctx context.Context, transitionID uuid.UUID) ([]Effect, error) {
	rows, err := s.q.ListWorkflowEffects(ctx, transitionID)
	if err != nil {
		return nil, err
	}
	out := make([]Effect, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToEffect(r))
	}
	return out, nil
}

func (s *Store) CreateWorkflowEffect(ctx context.Context, id, transitionID uuid.UUID, kind string, config json.RawMessage) (Effect, error) {
	row, err := s.q.CreateWorkflowEffect(ctx, dbgen.CreateWorkflowEffectParams{
		ID:           id,
		TransitionID: transitionID,
		Kind:         kind,
		Config:       config,
	})
	if err != nil {
		return Effect{}, err
	}
	return dbRowToEffect(row), nil
}

func (s *Store) ArchiveWorkflowEffect(ctx context.Context, id uuid.UUID, version int) (Effect, error) {
	row, err := s.q.ArchiveWorkflowEffect(ctx, dbgen.ArchiveWorkflowEffectParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Effect{}, err
	}
	return dbRowToEffect(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToStep(r dbgen.WorkflowStep) Step {
	s := Step{
		ID:        r.ID,
		Key:       r.Key,
		Name:      r.Name,
		NameEn:    r.NameEn,
		SortOrder: int(r.SortOrder),
		IsStart:   r.IsStart,
		IsEnd:     r.IsEnd,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		s.ArchivedAt = &t
	}
	return s
}

func dbRowToTransition(r dbgen.WorkflowTransition) Transition {
	t := Transition{
		ID:        r.ID,
		StepKey:   r.StepKey,
		FromKey:   r.FromKey,
		ToKey:     r.ToKey,
		Label:     r.Label,
		LabelEn:   r.LabelEn,
		SortOrder: int(r.SortOrder),
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		t.Archived = true
		at := r.ArchivedAt.Format(time.RFC3339)
		t.ArchivedAt = &at
	}
	return t
}

func dbRowToCondition(r dbgen.WorkflowCondition) Condition {
	c := Condition{
		ID:           r.ID,
		TransitionID: r.TransitionID,
		Kind:         r.Kind,
		Config:       r.Config,
		Version:      int(r.Version),
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		c.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		c.ArchivedAt = &t
	}
	return c
}

func dbRowToEffect(r dbgen.WorkflowEffect) Effect {
	e := Effect{
		ID:           r.ID,
		TransitionID: r.TransitionID,
		Kind:         r.Kind,
		Config:       r.Config,
		Version:      int(r.Version),
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		e.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		e.ArchivedAt = &t
	}
	return e
}
