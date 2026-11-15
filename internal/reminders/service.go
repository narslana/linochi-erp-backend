package reminders

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for reminders (REG-05).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ListReminders returns all active reminders (REG-05).
func (s *Service) ListReminders(ctx context.Context) ([]Reminder, error) {
	return s.store.ListReminders(ctx)
}

// CreateReminder creates a new reminder (REG-05).
func (s *Service) CreateReminder(ctx context.Context, idsEntryID, externalRefID uuid.UUID, reminderType, externalRefType, title, titleEn, description, dueDate string) (Reminder, error) {
	var reminder Reminder
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		reminder, err = s.store.CreateReminder(ctx, id, idsEntryID, externalRefID, reminderType, externalRefType, title, titleEn, description, dueDate)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "reminders",
			EntityType: "reminder",
			EntityID:   reminder.ID,
			Action:     "Create",
			After:      map[string]any{"title": title, "due_date": dueDate},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return reminder, err
}

// GetReminder returns a reminder by ID.
func (s *Service) GetReminder(ctx context.Context, id uuid.UUID) (Reminder, error) {
	reminder, err := s.store.GetReminder(ctx, id)
	if err != nil {
		return reminder, ProblemError(err)
	}
	return reminder, nil
}

// UpdateReminder updates a reminder (REG-05).
func (s *Service) UpdateReminder(ctx context.Context, title, titleEn, description *string, dueDate *string, id uuid.UUID, version int) (Reminder, error) {
	reminder, err := s.store.UpdateReminder(ctx, title, titleEn, description, dueDate, id, version)
	if err != nil {
		return reminder, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "reminders",
		EntityType: "reminder",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"title": title},
		Reason:     actor.ID.String(),
	})
	return reminder, nil
}

// ArchiveReminder archives a reminder (NFK-09).
func (s *Service) ArchiveReminder(ctx context.Context, id uuid.UUID, version int) (Reminder, error) {
	reminder, err := s.store.ArchiveReminder(ctx, id, version)
	if err != nil {
		return reminder, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "reminders",
		EntityType: "reminder",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"title": reminder.Title},
		Reason:     actor.ID.String(),
	})
	return reminder, nil
}

// MarkReminderNotified marks a reminder as notified (REG-05).
func (s *Service) MarkReminderNotified(ctx context.Context, id uuid.UUID, version int) (Reminder, error) {
	reminder, err := s.store.MarkReminderNotified(ctx, id, version)
	if err != nil {
		return reminder, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "reminders",
		EntityType: "reminder",
		EntityID:   id,
		Action:     "MarkNotified",
		Reason:     actor.ID.String(),
	})
	return reminder, nil
}

// ExpiredReminders returns reminders that have passed their due date (REG-05).
func (s *Service) ExpiredReminders(ctx context.Context) ([]Reminder, error) {
	return s.store.ExpiredReminders(ctx)
}
