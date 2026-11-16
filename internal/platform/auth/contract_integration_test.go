//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/boards"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/calendar"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/customers"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/ids"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/kiosk"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/orders"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/products"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/purchasing"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/reminders"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/shipments"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/stats"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/suppliers"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/workflow"
)

// specRouter loads api/openapi.yaml, the document the web app's client is
// generated from (NFK-29), and finds the operation a request belongs to.
func specRouter(t *testing.T) (*openapi3.T, func(*http.Request) (*openapi3filter.RequestValidationInput, error)) {
	t.Helper()
	doc, err := api.GetSpec()
	if err != nil {
		t.Fatalf("load the embedded spec: %v", err)
	}
	r, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatalf("build the spec router: %v", err)
	}
	return doc, func(req *http.Request) (*openapi3filter.RequestValidationInput, error) {
		route, params, err := r.FindRoute(req)
		if err != nil {
			return nil, err
		}
		return &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, nil
	}
}

// validate fails the test if the response is not one the spec describes for
// that operation: the status, the headers and the body.
func validate(t *testing.T, find func(*http.Request) (*openapi3filter.RequestValidationInput, error), req *http.Request, w *httptest.ResponseRecorder) {
	t.Helper()
	in, err := find(req)
	if err != nil {
		t.Fatalf("%s %s is not in the spec: %v", req.Method, req.URL.Path, err)
	}
	res := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: in, Status: w.Code, Header: w.Header(),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	res.SetBodyBytes(w.Body.Bytes())
	if err := openapi3filter.ValidateResponse(context.Background(), res); err != nil {
		t.Errorf("%s %s answered %d, which does not match the spec: %v\nheaders: %v\nbody: %s", req.Method, req.URL.Path, w.Code, err, w.Header(), w.Body.String())
	}
}

// recorded is a browser that records the request of every call, so that each
// answer can be checked against the spec afterwards.
type recorded struct {
	req *http.Request
	w   *httptest.ResponseRecorder
}

// contractEnv sets up the full composite handler with all modules for contract tests.
type contractEnv struct {
	pool   *pgxpool.Pool
	rec    *audit.Recorder
	sess   *auth.SessionStore
	mux    http.Handler
	idp    *oidctest.Server
	logger *slog.Logger
}

func newContractEnv(t *testing.T) *contractEnv {
	t.Helper()
	ctx := context.Background()

	server, err := testdb.StartServer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Terminate(ctx) })
	dsn := server.NewMigratedDatabase(t)
	appDSN := testdb.AppURL(dsn)

	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(ctx) })

	pool, err := db.Open(ctx, db.Options{URL: appDSN, MaxConns: 8, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	logs := &bytes.Buffer{}
	logger := obs.NewLogger(logs, slog.LevelWarn)
	rec, err := audit.NewRecorder(logger, metrics)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := auth.NewSessionStore(pool, rec, metrics, auth.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	mw := auth.NewSessionMiddleware(sessions)

	idp := oidctest.Start(t, providerCfg)

	authHandler, err := auth.NewHandler(auth.HandlerConfig{
		OIDC: auth.NewOIDCClient(auth.OIDCConfig{
			Issuer:       idp.Issuer(),
			ClientID:     providerCfg.ClientID,
			ClientSecret: providerCfg.ClientSecret,
			RedirectURL:  callbackURL,
		}, auth.WithOIDCHTTPClient(idp.Client()), auth.WithOIDCClock(time.Now), auth.WithOIDCLogger(logger)),
		Attempts:    auth.NewLoginAttemptStore(pool),
		Users:       auth.NewUserStore(pool, rec),
		Sessions:    sessions,
		AdminGroups: []string{"admins", "linochi_erp_users"},
		Version:     "1.2.3",
		Commit:      "abc1234",
		Logger:      logger,
		Metrics:     metrics,
		Now:         time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}

	boardsStore := boards.NewStore(pool)
	boardsSvc := boards.NewService(boardsStore, rec, pool)
	boardsH := boards.NewHandler(boardsSvc, mw, logger)

	productsStore := products.NewStore(pool)
	productsSvc := products.NewService(productsStore, rec, pool)
	productsH := products.NewHandler(productsSvc, mw, logger)

	idsStore := ids.NewStore(pool)
	idsSvc := ids.NewService(idsStore, rec, pool)
	idsH := ids.NewHandler(idsSvc, mw, logger)

	suppliersStore := suppliers.NewStore(pool)
	suppliersSvc := suppliers.NewService(suppliersStore, rec, pool)
	suppliersH := suppliers.NewHandler(suppliersSvc, mw, logger)

	purchasingStore := purchasing.NewStore(pool)
	purchasingSvc := purchasing.NewService(purchasingStore, rec, pool)
	purchasingH := purchasing.NewHandler(purchasingSvc, mw, logger)

	shipmentsStore := shipments.NewStore(pool)
	shipmentsSvc := shipments.NewService(shipmentsStore, rec, pool)
	shipmentsH := shipments.NewHandler(shipmentsSvc, mw, logger)

	inventoryStore := inventory.NewStore(pool)
	inventorySvc := inventory.NewService(inventoryStore, rec, pool)
	inventoryH := inventory.NewHandler(inventorySvc, mw, logger)

	calendarStore := calendar.NewStore(pool)
	calendarSvc := calendar.NewService(calendarStore, rec, pool)
	calendarH := calendar.NewHandler(calendarSvc, mw, logger)

	kioskStore := kiosk.NewStore(pool)
	kioskSvc := kiosk.NewService(kioskStore, rec, pool)
	kioskH := kiosk.NewHandler(kioskSvc, mw, logger)

	customersStore := customers.NewStore(pool)
	customersSvc := customers.NewService(customersStore, rec, pool)
	customersH := customers.NewHandler(customersSvc, mw, logger)

	workflowStore := workflow.NewStore(pool)
	workflowSvc := workflow.NewService(workflowStore, rec, pool)
	workflowH := workflow.NewHandler(workflowSvc, mw, logger)

	ordersStore := orders.NewStore(pool)
	ordersSvc := orders.NewService(ordersStore, rec, pool)
	ordersH := orders.NewHandler(ordersSvc, mw, logger)

	statsStore := stats.NewStore(pool)
	statsSvc := stats.NewService(statsStore)
	statsH := stats.NewHandler(statsSvc, mw)

	remindersStore := reminders.NewStore(pool)
	remindersSvc := reminders.NewService(remindersStore, rec, pool)
	remindersH := reminders.NewHandler(remindersSvc, mw, logger)

	composite := boards.NewComposite(
		authHandler, boardsH, productsH, idsH, suppliersH,
		purchasingH, shipmentsH, inventoryH, calendarH, kioskH,
		customersH, workflowH, ordersH, statsH, remindersH,
	)

	mux := http.NewServeMux()
	composite.Mount(mux)

	return &contractEnv{
		pool:   pool,
		rec:    rec,
		sess:   sessions,
		mux:    mux,
		idp:    idp,
		logger: logger,
	}
}

// contractBrowser is a cookie jar in front of the composite handler.
type contractBrowser struct {
	t       *testing.T
	mux     http.Handler
	cookies map[string]*http.Cookie
}

func (he *contractEnv) browser(t *testing.T) *contractBrowser {
	return &contractBrowser{t: t, mux: he.mux, cookies: map[string]*http.Cookie{}}
}

func (b *contractBrowser) call(method, target string, headers map[string]string, sink *[]recorded) *httptest.ResponseRecorder {
	b.t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), method, "https://erp.lab.internal"+target, nil)
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	b.mux.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	if sink != nil {
		*sink = append(*sink, recorded{r, w})
	}
	return w
}

func (he *contractEnv) callbackTarget(t *testing.T, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(he.idp.Authorize(t, authorizeURL))
	if err != nil {
		t.Fatal(err)
	}
	return u.RequestURI()
}

// TestNFK29_ResponsesMatchOpenAPI exercises every operation of the spec through the
// composite handler and asserts each response matches the spec (NFK-29).
func TestNFK29_ResponsesMatchOpenAPI(t *testing.T) {
	doc, find := specRouter(t)
	he := newContractEnv(t)
	var calls []recorded

	b := he.browser(t)
	// Not logged in.
	b.call("GET", "/api/v1/me", nil, &calls)
	b.call("GET", "/api/v1/version", nil, &calls)
	b.call("POST", "/auth/logout", map[string]string{"Sec-Fetch-Site": "cross-site"}, &calls)
	// A good login, with the answers of every step.
	w := b.call("GET", "/auth/login?return_to=%2Fproducts", nil, &calls)
	target := he.callbackTarget(t, w.Header().Get("Location"))
	b.call("GET", target, nil, &calls)
	b.call("GET", "/api/v1/me", nil, &calls)
	b.call("GET", "/api/v1/version", nil, &calls)
	b.call("POST", "/auth/logout", nil, &calls)
	b.call("POST", "/auth/logout", nil, &calls)
	// Refused logins: a state used twice, no state, a login Authelia turned down.
	b.call("GET", target, nil, &calls)
	b.call("GET", "/auth/callback", nil, &calls)
	he.idp.Configure(func(bh *oidctest.Behavior) { bh.DenyAuthorize = "access_denied" })
	w = b.call("GET", "/auth/login", nil, &calls)
	b.call("GET", he.callbackTarget(t, w.Header().Get("Location")), nil, &calls)
	// A login that cannot start.
	he.idp.Configure(func(bh *oidctest.Behavior) { bh.DenyAuthorize = ""; bh.DiscoveryStatus = http.StatusServiceUnavailable })
	b2 := he.browser(t)
	b2.call("GET", "/auth/login", nil, &calls)

	for _, c := range calls {
		validate(t, find, c.req, c.w)
	}

	// Every operation of the spec was exercised, and the ones that need a session
	// say so with 401 without one.
	seen := map[string]bool{}
	for _, c := range calls {
		in, err := find(c.req)
		if err != nil {
			t.Fatal(err)
		}
		seen[in.Route.Operation.OperationID] = true
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if !seen[op.OperationID] {
				t.Errorf("%s %s (%s) is in the spec and no call above exercised it", method, path, op.OperationID)
			}
		}
	}
}

// Whatever the spec says needs the session cookie must refuse a request that has
// none. This is read from the spec, so a new operation that is declared secured
// and not guarded by the handler fails here, and not in production.
func TestNFK29_EveryOperationThatNeedsASessionRefusesARequestWithoutOne(t *testing.T) {
	doc, find := specRouter(t)
	he := newContractEnv(t)
	b := he.browser(t)

	checked := 0
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			secured := false
			if op.Security != nil {
				for _, req := range *op.Security {
					if _, ok := req["session"]; ok {
						secured = true
					}
				}
			}
			if !secured {
				continue
			}
			checked++
			var sink []recorded
			w := b.call(method, path, nil, &sink)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s needs a session and answered %d without one", method, path, w.Code)
			}
			validate(t, find, sink[0].req, w)
		}
	}
	if checked == 0 {
		t.Error("no operation in the spec needs a session: this test checks nothing")
	}
}

// Every route the backend serves under /auth and /api is in the spec, and the
// other way round.
func TestNFK29_NothingIsServedThatTheSpecDoesNotDescribe(t *testing.T) {
	doc, _ := specRouter(t)
	he := newContractEnv(t)
	b := he.browser(t)
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			var sink []recorded
			w := b.call(method, path, nil, &sink)
			if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
				t.Errorf("%s %s is in the spec and the router does not serve it (%d)", method, path, w.Code)
			}
		}
	}
	// And a method the spec does not have is not served either.
	var sink []recorded
	if w := b.call("DELETE", "/api/v1/me", nil, &sink); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("DELETE /api/v1/me = %d", w.Code)
	}
}
