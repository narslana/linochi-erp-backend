package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/prometheus/client_golang/prometheus"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// LoginCookieName binds a login in progress to the browser that started it. It
// is a __Host- cookie like the session's, for the same reason, and it lives only
// as long as a login may take.
const LoginCookieName = "__Host-linochi_erp_login"

// HandlerConfig is what the login routes are made of.
type HandlerConfig struct {
	OIDC     *OIDCClient
	Attempts *LoginAttemptStore
	Users    *UserStore
	Sessions *SessionStore
	// AdminGroups are the Authelia groups that give the admin role.
	AdminGroups []string
	// Version and Commit are what GET /api/v1/version answers (INS-07).
	Version, Commit string
	Logger          *slog.Logger
	Metrics         *obs.Metrics
	// Now is the clock; time.Now when nil. For tests.
	Now func() time.Time
}

// Handler serves the auth routes of the OpenAPI document (NFK-10, NFK-11):
// /auth/login, /auth/callback, /auth/logout and /api/v1/me. It implements
// api.ServerInterface.
type Handler struct {
	cfg    HandlerConfig
	mw     *SessionMiddleware
	logins *prometheus.CounterVec
}

// Handler no longer implements api.ServerInterface directly.
// The boards.Composite wraps auth and boards handlers to satisfy the interface.

// NewHandler creates a Handler and registers linochi_auth_logins_total.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.OIDC == nil || cfg.Attempts == nil || cfg.Users == nil || cfg.Sessions == nil || cfg.Metrics == nil {
		return nil, errors.New("auth handler: OIDC, Attempts, Users, Sessions and Metrics are all required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	h := &Handler{
		cfg: cfg,
		mw:  NewSessionMiddleware(cfg.Sessions),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_auth_logins_total",
			Help: "Logins by result: ok, or the reason it was refused (no_role, mfa_missing, attempt_expired, and so on). A rise in anything but ok is what the refused-logins alert watches.",
		}, []string{"result"}),
	}
	if err := cfg.Metrics.Registry.Register(h.logins); err != nil {
		return nil, fmt.Errorf("register login metric: %w", err)
	}
	for _, r := range allReasons {
		h.logins.WithLabelValues(string(r))
	}
	return h, nil
}

// Mount registers all routes from the OpenAPI spec on the given mux. It is used
// by the tests; the production code uses boards.Composite instead. The auth
// handler implements api.ServerInterface so HandlerWithOptions can wire everything.
func (h *Handler) Mount(m api.ServeMux) {
	api.HandlerWithOptions(h, api.StdHTTPServerOptions{
		BaseRouter: m,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			switch err.(type) {
			case *api.InvalidParamFormatError, *api.RequiredParamError:
				httpx.Error(w, r, ErrUnauthenticated)
			default:
				httpx.Error(w, r, err)
			}
		},
	})
}

// Login starts a login: it stores the attempt, binds it to this browser with a
// cookie and sends the browser to Authelia (GET /auth/login).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request, params api.LoginParams) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")

	// The provider is checked before anything is stored, so that Authelia being
	// down leaves no rows behind.
	if err := h.cfg.OIDC.Ready(ctx); err != nil {
		h.refuse(w, r, err)
		return
	}
	returnTo := "/"
	if params.ReturnTo != nil {
		returnTo = SafeReturnTo(*params.ReturnTo)
	}
	pending, err := h.cfg.Attempts.Begin(ctx, returnTo)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	authorizeURL, err := h.cfg.OIDC.AuthCodeURL(ctx, pending.State, pending.Nonce, pending.Verifier)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	http.SetCookie(w, loginCookie(pending.Binding, int(LoginAttemptTTL/time.Second)))
	http.Redirect(w, r, authorizeURL, http.StatusSeeOther)
}

// LoginCallback finishes a login: it checks everything the provider says and
// only then makes a user and a session (GET /auth/callback). Every way it can
// fail ends in a redirect to the web app's error page with a fixed code.
func (h *Handler) LoginCallback(w http.ResponseWriter, r *http.Request, params api.LoginCallbackParams) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")

	// Whatever happens next, the login is over for this browser.
	binding := ""
	if c, err := r.Cookie(LoginCookieName); err == nil {
		binding = c.Value
	}
	http.SetCookie(w, loginCookie("", -1))

	state := deref(params.State)
	if state == "" {
		h.refuse(w, r, &LoginFailure{Reason: ReasonBadCallback, Err: errors.New("no state")})
		return
	}
	// The attempt is consumed before anything else is looked at, so every
	// callback is the last one for its state, good or bad.
	attempt, err := h.cfg.Attempts.Finish(ctx, state, binding)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	if e := deref(params.Error); e != "" {
		reason := ReasonProviderError
		if e == "access_denied" {
			reason = ReasonProviderDenied
		}
		h.refuse(w, r, &LoginFailure{Reason: reason, Err: fmt.Errorf("the provider answered %q", truncate(e, 64))})
		return
	}
	code := deref(params.Code)
	if code == "" {
		h.refuse(w, r, &LoginFailure{Reason: ReasonBadCallback, Err: errors.New("no code")})
		return
	}

	id, err := h.cfg.OIDC.Exchange(ctx, code, attempt.Verifier, attempt.Nonce)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	roles := RolesFromGroups(id.Groups, h.cfg.AdminGroups)
	if len(roles) == 0 {
		// Authelia let the person in, the app does not. Nothing is made for them.
		h.refuse(w, r, &LoginFailure{Reason: ReasonNoRole, Err: fmt.Errorf("none of %d groups gives a role", len(id.Groups))})
		return
	}
	user, err := h.cfg.Users.UpsertOnLogin(ctx, UserLogin{
		Issuer: id.Issuer, Subject: id.Subject, Email: id.Email, DisplayName: id.DisplayName,
	})
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	token, sess, err := h.cfg.Sessions.Create(ctx, user.ID, roles)
	if err != nil {
		h.refuse(w, r, err)
		return
	}

	http.SetCookie(w, SessionCookie(token, sess.ExpiresAt, h.cfg.Now()))
	h.logins.WithLabelValues(string(ReasonOK)).Inc()
	h.cfg.Logger.InfoContext(ctx, "login", slog.String("user_id", user.ID.String()), slog.String("session_id", sess.ID.String()))
	http.Redirect(w, r, SafeReturnTo(attempt.ReturnTo), http.StatusSeeOther)
}

// Logout ends the session (POST /auth/logout).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.mw.LogoutHandler().ServeHTTP(w, r)
}

// GetMe says who is logged in (GET /api/v1/me).
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.me)).ServeHTTP(w, r)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok {
		httpx.Error(w, r, ErrUnauthenticated)
		return
	}
	user, err := h.cfg.Users.Get(r.Context(), sess.UserID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	body := api.Me{Id: user.ID, Email: user.Email, DisplayName: user.DisplayName, Roles: make([]api.MeRoles, 0, len(sess.Roles))}
	for _, role := range sess.Roles {
		body.Roles = append(body.Roles, api.MeRoles(role))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// GetVersion says which release is running (GET /api/v1/version, INS-07).
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.version)).ServeHTTP(w, r)
}

func (h *Handler) version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.Version{Version: h.cfg.Version, Commit: h.cfg.Commit})
}

// refuse ends a login that did not work: it counts it, logs why with the trace
// id and sends the browser to the error page with a fixed code. The reason is
// worked out from the error, and nothing of it is in the response.
func (h *Handler) refuse(w http.ResponseWriter, r *http.Request, err error) {
	reason := reasonOf(err)
	h.logins.WithLabelValues(string(reason)).Inc()

	level := slog.LevelWarn
	if reason == ReasonInternal || reason == ReasonProviderUnavailable || reason == ReasonExchangeFailed ||
		reason == ReasonUserInfoFailed || reason == ReasonProviderError {
		level = slog.LevelError // not the person's doing: someone should look
	}
	h.cfg.Logger.Log(r.Context(), level, "login refused", slog.String("reason", string(reason)), slog.Any("error", err))

	http.Redirect(w, r, LoginErrorPath+"?code="+string(reason.Code()), http.StatusSeeOther)
}

// reasonOf is the reason a login ended with, from the error that ended it.
func reasonOf(err error) Reason {
	var lf *LoginFailure
	switch {
	case errors.As(err, &lf):
		return lf.Reason
	case errors.Is(err, ErrLoginAttemptUnknown):
		return ReasonAttemptUnknown
	case errors.Is(err, ErrLoginAttemptExpired):
		return ReasonAttemptExpired
	case errors.Is(err, ErrLoginBindingMismatch):
		return ReasonBindingMismatch
	case errors.Is(err, ErrTooManyLoginAttempts):
		return ReasonTooManyAttempts
	case errors.Is(err, ErrUserArchived):
		return ReasonUserArchived
	}
	return ReasonInternal
}

// loginCookie is the one place the binding cookie is built, as for the session:
// __Host- needs Secure, Path=/ and no Domain. Lax, and not Strict, because the
// callback is a top-level navigation from Authelia, which Strict would not send
// the cookie on.
func loginCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: LoginCookieName, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Stub methods for ServerInterface - non-auth routes return 200 OK.
// Real implementations delegate through boards.Composite in production.

func (h *Handler) AcceptOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveCustomer(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID, params api.ArchiveICalSourceParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID, params api.ArchiveKioskDeviceParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, params api.ArchiveKioskPlaylistParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveLead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchivePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID, params api.ArchivePlaylistViewParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchivePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchivePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveReminder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, spId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ArchiveWorkflowTransition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) AssignFieldToProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) BatchExportOrders(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateCustomerContact(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateFieldDefinition(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateICalSource(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateIdsEntry(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateKioskDevice(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateKioskPlaylist(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateLead(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateOrderLine(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateProductType(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateStockMovement(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateWorkflowStep(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateWorkflowTransition(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateWorkflowTransitionCondition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) CreateWorkflowTransitionEffect(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) DeleteOrderLine(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, lineId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ExportOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetCalendarEvent(w http.ResponseWriter, r *http.Request, eventId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetFEFOBatches(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, locationId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetInventoryStats(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetInventoryValue(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetKeyFigures(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetKioskData(w http.ResponseWriter, r *http.Request, params api.GetKioskDataParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetLead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetReminder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetRevenue(w http.ResponseWriter, r *http.Request, params api.GetRevenueParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetStatsCards(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) GetWorkflowTransition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListBoards(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCalendarEvents(w http.ResponseWriter, r *http.Request, params api.ListCalendarEventsParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListChecklistItems(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListColumns(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCustomerContacts(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCustomerEANSeries(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCustomerLeads(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request, params api.ListCustomersParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListEANSeries(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListFieldDefinitions(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListICalSources(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListIdsEntries(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListKioskDevices(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListKioskPlaylists(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListLeads(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListOrderLines(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request, params api.ListOrdersParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListPlaylistViews(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListProductDocuments(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListProductImages(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListProductTypeFields(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListProductTypes(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request, params api.ListProductsParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListPurchaseOrderLines(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListPurchaseOrders(w http.ResponseWriter, r *http.Request, params api.ListPurchaseOrdersParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListReminders(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListShipments(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListShipmentsByPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListStockMovements(w http.ResponseWriter, r *http.Request, params api.ListStockMovementsParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListSupplierContacts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListSupplierProducts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListWorkflowSteps(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListWorkflowTransitionConditions(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListWorkflowTransitionEffects(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ListWorkflowTransitions(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) LockProductSKU(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) MarkReminderNotified(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) PairKioskDevice(w http.ResponseWriter, r *http.Request, params api.PairKioskDeviceParams) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) RegisterCustomerEANSeries(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) RegisterEANSeries(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ReleaseStock(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ReserveStock(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) ShipOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) SyncCalendar(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateLead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateOrderLine(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, lineId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdatePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateReminder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateWorkflowStep(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) UpdateWorkflowTransition(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	httpx.Error(w, r, ErrUnauthenticated)
}
func (h *Handler) WithdrawStock(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, ErrUnauthenticated)
}
