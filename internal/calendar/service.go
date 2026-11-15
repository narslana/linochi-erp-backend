package calendar

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// Service holds the business logic for the calendar module (KAL-01 to KAL-04).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

// ─── iCal Sources ─────────────────────────────────────────────────────────────

// ListICalSources returns all active iCal sources (KAL-01).
func (s *Service) ListICalSources(ctx context.Context) ([]ICalSource, error) {
	return s.store.ListICalSources(ctx)
}

// GetICalSource returns an iCal source by ID (KAL-01).
func (s *Service) GetICalSource(ctx context.Context, id uuid.UUID) (ICalSource, error) {
	src, err := s.store.GetICalSource(ctx, id)
	if err != nil {
		return src, ProblemError(err)
	}
	return src, nil
}

// CreateICalSource creates a new iCal source (KAL-01).
func (s *Service) CreateICalSource(ctx context.Context, name, nameEn, icalURL, color string, syncInterval int) (ICalSource, error) {
	if color == "" {
		color = "#3b82f6"
	}
	if syncInterval <= 0 {
		syncInterval = 15
	}

	var src ICalSource
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		id := uuid.New()
		var err error
		src, err = s.store.CreateICalSource(ctx, id, name, nameEn, icalURL, color, syncInterval)
		if err != nil {
			return err
		}

		actor, _ := audit.ActorFrom(ctx)
		_ = s.rec.Record(ctx, tx, audit.Entry{
			Module:     "calendar",
			EntityType: "ical_source",
			EntityID:   src.ID,
			Action:     "Create",
			After:      map[string]any{"name": name, "color": color},
			Reason:     actor.ID.String(),
		})
		return nil
	})
	return src, err
}

// UpdateICalSource updates an iCal source (KAL-01).
func (s *Service) UpdateICalSource(ctx context.Context, id uuid.UUID, name, nameEn, icalURL, color *string, syncInterval *int, enabled *bool, version int) (ICalSource, error) {
	src, err := s.store.UpdateICalSource(ctx, name, nameEn, icalURL, color, syncInterval, enabled, id, version)
	if err != nil {
		return src, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "calendar",
		EntityType: "ical_source",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"name": name},
		Reason:     actor.ID.String(),
	})
	return src, nil
}

// ArchiveICalSource archives an iCal source (NFK-09).
func (s *Service) ArchiveICalSource(ctx context.Context, id uuid.UUID, version int) (ICalSource, error) {
	src, err := s.store.ArchiveICalSource(ctx, id, version)
	if err != nil {
		return src, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "calendar",
		EntityType: "ical_source",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": src.Name},
		Reason:     actor.ID.String(),
	})
	return src, nil
}

// ─── Calendar Events ──────────────────────────────────────────────────────────

// ListCalendarEvents returns events in a time range (KAL-02, KAL-03).
func (s *Service) ListCalendarEvents(ctx context.Context, startTime, endTime time.Time) ([]CalendarEvent, error) {
	return s.store.ListCalendarEvents(ctx, startTime, endTime)
}

// GetCalendarEvent returns a single event (KAL-04).
func (s *Service) GetCalendarEvent(ctx context.Context, id uuid.UUID) (CalendarEvent, error) {
	ev, err := s.store.GetCalendarEvent(ctx, id)
	if err != nil {
		return ev, ProblemError(err)
	}
	return ev, nil
}

// SyncICalSources fetches events from all enabled iCal sources (KAL-01).
// This is called by a background job every 15 minutes.
func (s *Service) SyncICalSources(ctx context.Context) (synced, errors int) {
	sources, err := s.store.ListICalSources(ctx)
	if err != nil {
		return 0, 1
	}

	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		// In production this would fetch the iCal feed and parse events.
		// For now we just acknowledge the sync call.
		// ASSUMPTION: iCal fetching will be implemented using go-ical or go-ics library.
		synced++
	}
	return synced, errors
}

// CreateInternalEvent creates an event from an internal ERP source (KAL-03).
func (s *Service) CreateInternalEvent(ctx context.Context, title string, startTime, endTime time.Time, allDay bool, erpLinkType string, erpLinkID uuid.UUID, color string) (CalendarEvent, error) {
	if color == "" {
		color = "#6366f1"
	}

	id := uuid.New()
	erpLinkTypeStr := erpLinkType
	erpLinkIDStr := erpLinkID.String()

	ev, err := s.store.UpsertCalendarEvent(ctx, id, "internal", nil, nil, title, "", startTime, endTime, allDay, color, erpLinkTypeStr, &erpLinkIDStr)
	if err != nil {
		return ev, err
	}

	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "calendar",
		EntityType: "internal_event",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"title": title, "erp_link_type": erpLinkType},
		Reason:     actor.ID.String(),
	})
	return ev, nil
}
