// Package workflow is the order workflow engine (WF-01 to WF-07, WF-10).
//
// The flow is data: versioned steps and transitions in the database. Nothing in
// this package tests a status name. What happens to stock when an order changes
// step follows from the stock phase of the two steps (none, reserved, shipped),
// and a published version is validated to keep its effects consistent with them.
package workflow

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// Stock phases: what the stock of an order looks like in a step.
const (
	PhaseNone     = "none"
	PhaseReserved = "reserved"
	PhaseShipped  = "shipped"
)

// Effect kinds. The first three can be attached to a transition (WF-03); return
// only appears when an override moves an order back from a shipped step.
const (
	EffectReserve = "reserve"
	EffectRelease = "release"
	EffectShip    = "ship"
	EffectReturn  = "return"
)

// WarningStockNotAvailable is returned when an admin reopens a cancelled or
// rejected order and the stock for it could not be reserved.
const WarningStockNotAvailable = "stock_not_available"

// Actor is who asks for a status change. Admin comes from the session roles.
type Actor struct {
	UserID uuid.UUID
	Admin  bool
}

// StepDef is a step in a flow definition.
type StepDef struct {
	Key        string `json:"key"`
	SortOrder  int    `json:"sort_order"`
	IsStart    bool   `json:"is_start"`
	IsFinal    bool   `json:"is_final"`
	IsLocked   bool   `json:"is_locked"`
	StockPhase string `json:"stock_phase"` // "" means none
}

// TransitionDef is an allowed move between two steps, by key.
type TransitionDef struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Effect string `json:"effect,omitempty"`
}

// VersionDefinition is what is published: a complete flow.
type VersionDefinition struct {
	Steps       []StepDef       `json:"steps"`
	Transitions []TransitionDef `json:"transitions"`
}

// Step is a stored step.
type Step struct {
	ID uuid.UUID
	StepDef
}

// Version is a published flow version with its steps and transitions.
type Version struct {
	ID          uuid.UUID       `json:"id"`
	WorkflowID  uuid.UUID       `json:"workflow_id"`
	Version     int             `json:"version"`
	PublishedAt time.Time       `json:"published_at"`
	IsActive    bool            `json:"is_active"`
	Steps       []StepDef       `json:"steps"`
	Transitions []TransitionDef `json:"transitions"`
}

// Definition returns the flow as a definition, to publish a changed copy of it.
func (v Version) Definition() VersionDefinition {
	return VersionDefinition{
		Steps:       append([]StepDef(nil), v.Steps...),
		Transitions: append([]TransitionDef(nil), v.Transitions...),
	}
}

// PlannedEffect is something a status change does to stock (WF-05 preview).
type PlannedEffect struct {
	Kind        string    `json:"kind"`
	OrderLineID uuid.UUID `json:"order_line_id"`
	ProductID   uuid.UUID `json:"product_id"`
	Quantity    int       `json:"quantity"`
}

// Result is what a status change did.
type Result struct {
	OrderID  uuid.UUID       `json:"order_id"`
	FromKey  string          `json:"from_key"`
	ToKey    string          `json:"to_key"`
	Effects  []PlannedEffect `json:"effects"`
	Warnings []string        `json:"warnings"`
}

// HistoryEntry is one row of the order's status history (WF-10).
type HistoryEntry struct {
	ID         uuid.UUID `json:"id"`
	OrderID    uuid.UUID `json:"order_id"`
	FromKey    string    `json:"from_key"`
	ToKey      string    `json:"to_key"`
	ActorID    uuid.UUID `json:"actor_id"`
	Reason     string    `json:"reason"`
	IsOverride bool      `json:"is_override"`
	CreatedAt  time.Time `json:"created_at"`
}

// StepTime is the time an order spent in a step, summed over visits (WF-10).
type StepTime struct {
	Key      string        `json:"key"`
	Duration time.Duration `json:"duration"`
}

// ─── Typed errors, mapped to RFC 9457 by httpx ───────────────────────────────

// TransitionNotAllowedError: the move is not in the order's flow (WF-02).
type TransitionNotAllowedError struct{ From, To string }

func (e *TransitionNotAllowedError) Error() string {
	return fmt.Sprintf("transition %q to %q is not allowed", e.From, e.To)
}

func (e *TransitionNotAllowedError) Problem() httpx.Problem {
	return httpx.Problem{Code: "transition-not-allowed", Title: "Transition not allowed", Status: http.StatusConflict, Detail: e.Error()}
}

// FinalStateError: nothing can move an order out of a locked step (delivered).
type FinalStateError struct{ Key string }

func (e *FinalStateError) Error() string { return fmt.Sprintf("an order in %q cannot be moved", e.Key) }

func (e *FinalStateError) Problem() httpx.Problem {
	return httpx.Problem{Code: "order-final", Title: "The order is in a final state", Status: http.StatusConflict, Detail: e.Error()}
}

// ForbiddenError: an override needs the admin role (WF-05).
type ForbiddenError struct{}

func (*ForbiddenError) Error() string { return "a manual override needs the admin role" }

func (e *ForbiddenError) Problem() httpx.Problem {
	return httpx.Problem{Code: "override-forbidden", Title: "Override not allowed", Status: http.StatusForbidden, Detail: e.Error()}
}

// ReasonRequiredError: an override needs a reason (WF-05).
type ReasonRequiredError struct{}

func (*ReasonRequiredError) Error() string { return "a manual override needs a reason" }

func (e *ReasonRequiredError) Problem() httpx.Problem {
	return httpx.Problem{Code: "reason-required", Title: "A reason is required", Status: http.StatusUnprocessableEntity, Detail: e.Error()}
}

// InvalidFlowError: the flow breaks a rule of WF-06 and cannot be published.
type InvalidFlowError struct{ Problems []string }

func (e *InvalidFlowError) Error() string { return "invalid flow: " + strings.Join(e.Problems, "; ") }

func (e *InvalidFlowError) Problem() httpx.Problem {
	return httpx.Problem{Code: "invalid-flow", Title: "Invalid workflow", Status: http.StatusUnprocessableEntity, Detail: strings.Join(e.Problems, "; ")}
}
