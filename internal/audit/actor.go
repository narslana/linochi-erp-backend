package audit

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ActorType says what kind of actor made a change.
type ActorType string

const (
	// ActorUser is a logged-in person (the user's id).
	ActorUser ActorType = "user"
	// ActorAPIKey is an integration such as n8n (the API key's id).
	ActorAPIKey ActorType = "api_key"
	// ActorSystem is the app itself, for example the login flow or a background
	// job. It is named by a label, never by a person.
	ActorSystem ActorType = "system"
)

// Actor is who or what made a change.
type Actor struct {
	Type  ActorType
	ID    uuid.UUID // zero for ActorSystem
	Label string    // only for ActorSystem, for example "oidc-login"
}

// UserActor is a logged-in user.
func UserActor(id uuid.UUID) Actor { return Actor{Type: ActorUser, ID: id} }

// APIKeyActor is an API key.
func APIKeyActor(id uuid.UUID) Actor { return Actor{Type: ActorAPIKey, ID: id} }

// SystemActor is the app itself. The label names the part of the system.
func SystemActor(label string) Actor { return Actor{Type: ActorSystem, Label: label} }

// ErrNoActor means a change was recorded without an actor in the context. The
// transaction fails and is rolled back: an unattributed change is not saved.
var ErrNoActor = errors.New("audit: no actor in the context")

type actorKey struct{}

// WithActor returns a context carrying the actor. The auth middleware sets it
// for requests (B5/B6, and for API keys in phase 6), background jobs set a
// system actor.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom returns the actor set by WithActor.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}
