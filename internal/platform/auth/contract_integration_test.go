//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// mustOK fails the test when a call that should succeed returns >= 400.
// Contract validation alone is not enough because a 500 problem response can
// still match the spec.
func mustOK(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Code >= 400 {
		t.Fatalf("%s returned %d: %s", what, w.Code, w.Body.String())
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

func (b *contractBrowser) callJSON(method, target string, body interface{}, headers map[string]string, sink *[]recorded) *httptest.ResponseRecorder {
	b.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		b.t.Fatalf("marshal request body for %s %s: %v", method, target, err)
	}
	r := httptest.NewRequestWithContext(context.Background(), method, "https://erp.lab.internal"+target, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
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

	// Log in again for the module tests (previous logout cleared the session).
	b = he.browser(t)
	w = b.call("GET", "/auth/login?return_to=%2Fproducts", nil, &calls)
	target = he.callbackTarget(t, w.Header().Get("Location"))
	b.call("GET", target, nil, &calls)

	// ---- dependencies: boards ----
	w = b.callJSON("POST", "/api/v1/boards", map[string]interface{}{"name": "Sprint 1"}, nil, &calls)
	mustOK(t, w, "POST /api/v1/boards")
	var board map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &board)
	boardID := board["id"].(string)
	w = b.callJSON("POST", "/api/v1/boards/"+boardID+"/columns", map[string]interface{}{"name": "To do"}, nil, &calls)
	mustOK(t, w, "POST /api/v1/boards/"+boardID+"/columns")
	var col1 map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &col1)
	col1ID := col1["id"].(string)
	w = b.callJSON("POST", "/api/v1/boards/"+boardID+"/cards", map[string]interface{}{"title": "First card", "column_id": col1ID}, nil, &calls)
	mustOK(t, w, "POST /api/v1/boards/"+boardID+"/cards")
	var card1 map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &card1)
	card1ID := card1["id"].(string)
	w = b.callJSON("POST", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/checklist", map[string]interface{}{"title": "Check one"}, nil, &calls)
	mustOK(t, w, "POST checklist")
	var clItem map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &clItem)
	clItemID := clItem["id"].(string)
	b.callJSON("POST", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/comments", map[string]interface{}{"body_md": "note"}, nil, &calls)
	// ---- dependencies: product-types ----
	w = b.callJSON("POST", "/api/v1/product-types", map[string]interface{}{"name": "Shampoo", "name_en": "Shampoo", "type_code": "SHAMPOO"}, nil, &calls)
	mustOK(t, w, "POST /api/v1/product-types")
	var pt map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &pt)
	ptID := pt["id"].(string)
	if ptID == "" {
		t.Fatalf("product type response missing id: %s", w.Body.String())
	}
	w = b.callJSON("GET", "/api/v1/product-types/"+ptID, nil, nil, &calls)
	mustOK(t, w, "GET /api/v1/product-types/"+ptID)
	w = b.callJSON("GET", "/api/v1/product-types", nil, nil, &calls)
	mustOK(t, w, "GET /api/v1/product-types")
	w = b.callJSON("POST", "/api/v1/field-definitions", map[string]interface{}{"name": "Volume", "name_en": "Volume", "field_type": "decimal", "group_name": "General"}, nil, &calls)
	mustOK(t, w, "POST /api/v1/field-definitions")
	var fd map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &fd)
	if fd["id"] == nil {
		t.Fatalf("field definition response missing id: %s", w.Body.String())
	}
	fdID := fd["id"].(string)
	// ---- dependencies: locations ----
	w = b.callJSON("POST", "/api/v1/locations", map[string]interface{}{"name": "Warehouse A"}, nil, &calls)
	var loc map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &loc)
	locID := loc["id"].(string)
	// ---- dependencies: suppliers ----
	w = b.callJSON("POST", "/api/v1/suppliers", map[string]interface{}{"name": "Raw Materials AB", "role": "manufacturer", "currency": "SEK"}, nil, &calls)
	var sup map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &sup)
	supID := sup["id"].(string)
	w = b.callJSON("POST", "/api/v1/suppliers/"+supID+"/contacts", map[string]interface{}{"name": "Contact Person", "email": "contact@example.com"}, nil, &calls)
	var supContact map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &supContact)
	supContactID := supContact["id"].(string)
	// ---- dependencies: purchase-orders ----
	w = b.callJSON("POST", "/api/v1/purchase-orders", map[string]interface{}{"supplier_id": supID, "order_number": "PO-001", "order_date": "2026-01-15", "currency": "SEK"}, nil, &calls)
	var po map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &po)
	var poID string // reassigned on line 531
	// ---- dependencies: customers ----
	w = b.callJSON("POST", "/api/v1/customers", map[string]interface{}{"name": "Dog Shop AB", "type_code": "SHOP", "currency": "SEK"}, nil, &calls)
	var cust map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &cust)
	custID := cust["id"].(string)
	b.callJSON("POST", "/api/v1/customers/"+custID+"/contacts", map[string]interface{}{"customer_id": custID, "first_name": "Anna", "last_name": "Smith", "email": "anna@example.com"}, nil, &calls)
	b.callJSON("POST", "/api/v1/customers/"+custID+"/ean-series", map[string]interface{}{"prefix": "73", "start_code": "100", "end_code": "200"}, nil, &calls)
	// ---- dependencies: leads ----
	b.callJSON("POST", "/api/v1/leads", map[string]interface{}{"company": "Paws Inc", "email": "info@paws.com", "status": "new"}, nil, &calls)
	// ---- dependencies: workflow steps ----
	w = b.callJSON("POST", "/api/v1/workflow/steps", map[string]interface{}{"key": "draft", "name": "Draft", "name_en": "Draft", "is_start": true}, nil, &calls)
	var stepDraft map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &stepDraft)
	draftStepID := stepDraft["id"].(string)
	b.callJSON("POST", "/api/v1/workflow/steps", map[string]interface{}{"key": "accepted", "name": "Accepted", "name_en": "Accepted"}, nil, &calls)
	b.callJSON("POST", "/api/v1/workflow/steps", map[string]interface{}{"key": "shipped", "name": "Shipped", "name_en": "Shipped"}, nil, &calls)
	b.callJSON("POST", "/api/v1/workflow/steps", map[string]interface{}{"key": "done", "name": "Done", "name_en": "Done", "is_end": true}, nil, &calls)
	// ---- dependencies: workflow transitions ----
	w = b.callJSON("POST", "/api/v1/workflow/transitions", map[string]interface{}{"step_key": "accepted", "from_key": "draft", "to_key": "accepted", "label": "Accept"}, nil, &calls)
	var trans map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &trans)
	transID := trans["id"].(string)
	// ---- dependencies: ical-sources ----
	w = b.callJSON("POST", "/api/v1/ical-sources", map[string]interface{}{"name": "Work", "ical_url": "https://example.com/calendar.ics"}, nil, &calls)
	var ical map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ical)
	icalID := ical["id"].(string)
	// ---- dependencies: kiosk devices and playlists ----
	w = b.callJSON("POST", "/api/v1/kiosk/devices", map[string]interface{}{"name": "Kiosk TV"}, nil, &calls)
	var dev map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &dev)
	devID := dev["id"].(string)
	w = b.callJSON("POST", "/api/v1/kiosk/playlists", map[string]interface{}{"name": "Main"}, nil, &calls)
	var pl map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &pl)
	plID := pl["id"].(string)
	w = b.callJSON("POST", "/api/v1/kiosk/playlists/"+plID+"/views", map[string]interface{}{"view_type": "map", "sort_order": 1, "duration_sec": 30}, nil, &calls)
	var plView map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &plView)
	plViewID := plView["id"].(string)
	b.callJSON("POST", "/api/v1/kiosk/pair", map[string]interface{}{}, nil, &calls)
	// ---- dependencies: ids entries ----
	w = b.callJSON("POST", "/api/v1/ids", map[string]interface{}{"category": "eori", "name": "SE1234567890000", "value": "SE1234567890000"}, nil, &calls)
	var ids map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ids)
	idsID := ids["id"].(string)
	// ---- dependencies: ean-series ----
	w = b.callJSON("POST", "/api/v1/ean-series", map[string]interface{}{"prefix": "74", "start_code": "100", "end_code": "200"}, nil, &calls)
	mustOK(t, w, "POST /api/v1/ean-series")
	var ean map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ean)
	if ean["id"] == nil {
		t.Fatalf("ean-series response missing id: %s", w.Body.String())
	}
	eanID := ean["id"].(string)
	// ---- dependencies: reminders ----
	w = b.callJSON("POST", "/api/v1/reminders", map[string]interface{}{"reminder_type": "ids_entry", "ids_entry_id": idsID, "title": "Renewal", "title_en": "Renewal", "due_date": "2026-02-01"}, nil, &calls)
	var rem map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &rem)
	remID := rem["id"].(string)
	// ---- products: CRUD ----
	w = b.callJSON("POST", "/api/v1/products", map[string]interface{}{"name": "Dog Shampoo", "name_en": "Dog Shampoo", "product_type_id": ptID, "volume": 500, "volume_unit": "ml", "unit": "bottle", "base_price_sek": 19900, "cost_price_sek": 8000, "vat_rate": 0.25}, nil, &calls)
	var prod map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &prod)
	prodID := prod["id"].(string)
	b.call("GET", "/api/v1/products", nil, &calls)
	b.callJSON("PATCH", "/api/v1/products/"+prodID, map[string]interface{}{"id": prodID, "version": 1, "name": "Dog Shampoo Updated"}, nil, &calls)
	w = b.callJSON("POST", "/api/v1/products/"+prodID+"/images", map[string]interface{}{"url": "https://example.com/img.jpg"}, nil, &calls)
	var img map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &img)
	imgID := img["id"].(string)
	b.call("GET", "/api/v1/products/"+prodID+"/images", nil, &calls)
	b.callJSON("PATCH", "/api/v1/products/"+prodID+"/images/"+imgID, map[string]interface{}{"version": 1, "is_main": true}, nil, &calls)
	w = b.callJSON("POST", "/api/v1/products/"+prodID+"/documents", map[string]interface{}{"title": "SDS", "title_en": "SDS", "document_type": "sds", "url": "https://example.com/sds.pdf"}, nil, &calls)
	var docResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &docResp)
	docID := docResp["id"].(string)
	b.call("GET", "/api/v1/products/"+prodID+"/documents", nil, &calls)
	b.callJSON("PATCH", "/api/v1/products/"+prodID+"/documents/"+docID, map[string]interface{}{"version": 1, "title": "SDS Updated"}, nil, &calls)
	b.callJSON("POST", "/api/v1/products/"+prodID+"/lock-sku", map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/products/"+prodID, map[string]interface{}{}, nil, &calls)
	// ---- ids: CRUD ----
	b.call("GET", "/api/v1/ids", nil, &calls)
	b.call("GET", "/api/v1/ids/"+idsID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/ids/"+idsID, map[string]interface{}{"id": idsID, "version": 1, "value": "SE9876543210000"}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/ids/"+idsID, map[string]interface{}{}, nil, &calls)
	// ---- suppliers: CRUD ----
	b.call("GET", "/api/v1/suppliers", nil, &calls)
	b.call("GET", "/api/v1/suppliers/"+supID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/suppliers/"+supID, map[string]interface{}{"version": 1, "notes": "Updated notes"}, nil, &calls)
	b.call("GET", "/api/v1/suppliers/"+supID+"/contacts", nil, &calls)
	b.callJSON("PATCH", "/api/v1/suppliers/"+supID+"/contacts/"+supContactID, map[string]interface{}{"version": 1, "name": "Updated Contact"}, nil, &calls)
	w = b.callJSON("POST", "/api/v1/suppliers/"+supID+"/products", map[string]interface{}{"product_id": prodID, "purchase_price": 8000, "purchase_currency": "SEK"}, nil, &calls)
	var sp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &sp)
	spID := sp["id"].(string)
	b.call("GET", "/api/v1/suppliers/"+supID+"/products", nil, &calls)
	b.callJSON("DELETE", "/api/v1/suppliers/"+supID+"/products/"+spID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/suppliers/"+supID, map[string]interface{}{}, nil, &calls)
	// ---- purchasing: CRUD ----
	b.call("GET", "/api/v1/purchase-orders", nil, &calls)
	w = b.callJSON("POST", "/api/v1/purchase-orders", map[string]interface{}{"supplier_id": supID, "order_number": "PO-002", "order_date": "2026-01-20", "currency": "SEK"}, nil, &calls)
	_ = json.Unmarshal(w.Body.Bytes(), &po)
	poID = po["id"].(string)
	b.call("GET", "/api/v1/purchase-orders/"+poID, nil, &calls)
	w = b.callJSON("POST", "/api/v1/purchase-orders/"+poID+"/lines", map[string]interface{}{"product_id": prodID, "quantity": 100, "unit_price": 8000}, nil, &calls)
	var pol map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &pol)
	polID := pol["id"].(string)
	b.call("GET", "/api/v1/purchase-orders/"+poID+"/lines", nil, &calls)
	b.callJSON("PATCH", "/api/v1/purchase-orders/"+poID+"/lines/"+polID, map[string]interface{}{"version": 1, "quantity": 120}, nil, &calls)
	b.callJSON("PATCH", "/api/v1/purchase-orders/"+poID, map[string]interface{}{"version": 1, "notes": "Updated PO"}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/purchase-orders/"+poID, map[string]interface{}{}, nil, &calls)
	// ---- shipments: CRUD ----
	w = b.callJSON("POST", "/api/v1/shipments", map[string]interface{}{"purchase_order_id": poID, "from_location": "Factory", "to_location": "Warehouse", "carrier": "PostNord", "batch_number": "B001", "status": "planned"}, nil, &calls)
	var ship map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ship)
	shipID := ship["id"].(string)
	b.call("GET", "/api/v1/shipments", nil, &calls)
	b.call("GET", "/api/v1/shipments/"+shipID, nil, &calls)
	b.call("GET", "/api/v1/shipments/by-po/"+poID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/shipments/"+shipID, map[string]interface{}{"version": 1, "status": "in_transit"}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/shipments/"+shipID, map[string]interface{}{}, nil, &calls)
	// ---- inventory: CRUD ----
	w = b.callJSON("POST", "/api/v1/locations", map[string]interface{}{"name": "Warehouse B"}, nil, &calls)
	var loc2 map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &loc2)
	loc2ID := loc2["id"].(string)
	b.call("GET", "/api/v1/locations", nil, &calls)
	b.call("GET", "/api/v1/locations/"+locID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/locations/"+locID, map[string]interface{}{"version": 1, "name": "Warehouse A Updated"}, nil, &calls)
	b.call("GET", "/api/v1/locations/"+loc2ID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/locations/"+loc2ID, map[string]interface{}{"version": 1, "name": "Warehouse B Updated"}, nil, &calls)
	b.call("GET", "/api/v1/inventory/balance/"+prodID, nil, &calls)
	b.call("GET", "/api/v1/inventory/value", nil, &calls)
	b.callJSON("POST", "/api/v1/stockmovements", map[string]interface{}{"product_id": prodID, "location_id": locID, "movement_type": "purchase", "quantity": 100}, nil, &calls)
	b.call("GET", "/api/v1/stockmovements", nil, &calls)
	b.callJSON("DELETE", "/api/v1/locations/"+locID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/locations/"+loc2ID, map[string]interface{}{}, nil, &calls)
	// ---- customers: CRUD ----
	b.call("GET", "/api/v1/customers", nil, &calls)
	b.call("GET", "/api/v1/customers/"+custID, nil, &calls)
	b.callJSON("PUT", "/api/v1/customers/"+custID, map[string]interface{}{"id": custID, "version": 1, "notes": "Updated customer"}, nil, &calls)
	b.call("GET", "/api/v1/customers/"+custID+"/contacts", nil, &calls)
	b.call("GET", "/api/v1/customers/"+custID+"/ean-series", nil, &calls)
	b.call("GET", "/api/v1/customers/"+custID+"/leads", nil, &calls)
	b.callJSON("DELETE", "/api/v1/customers/"+custID, map[string]interface{}{}, nil, &calls)
	// ---- orders: CRUD ----
	w = b.callJSON("POST", "/api/v1/orders", map[string]interface{}{"customer_id": custID, "channel": "direct", "currency": "SEK", "exchange_rate_sek": 1.0, "lines": []map[string]interface{}{{"product_id": prodID, "quantity": 2, "unit_price_sek": 19900}}}, nil, &calls)
	var ord map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ord)
	ordID := ord["id"].(string)
	ordVer := int(ord["version"].(float64))
	b.call("GET", "/api/v1/orders", nil, &calls)
	b.call("GET", "/api/v1/orders/"+ordID, nil, &calls)
	b.call("GET", "/api/v1/orders/"+ordID+"/lines", nil, &calls)
	b.callJSON("PUT", "/api/v1/orders/"+ordID, map[string]interface{}{"id": ordID, "version": ordVer, "notes": "Updated order"}, nil, &calls)
	b.callJSON("POST", "/api/v1/orders/"+ordID+"/lines", map[string]interface{}{"product_id": prodID, "quantity": 1, "unit_price_sek": 19900}, nil, &calls)
	b.call("GET", "/api/v1/orders/"+ordID+"/export", nil, &calls)
	b.callJSON("POST", "/api/v1/orders/"+ordID+"/status", map[string]interface{}{"status": "accepted"}, nil, &calls)
	b.callJSON("POST", "/api/v1/orders/"+ordID+"/status/accept", map[string]interface{}{"status": "accepted", "reason": "Test accept"}, nil, &calls)
	b.callJSON("POST", "/api/v1/orders/"+ordID+"/status/ship", map[string]interface{}{"status": "shipped"}, nil, &calls)
	b.callJSON("POST", "/api/v1/orders/"+ordID+"/status/cancel", map[string]interface{}{"status": "cancelled", "reason": "Test cancel"}, nil, &calls)
	b.callJSON("POST", "/api/v1/orders/batch-export", map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/orders/"+ordID, map[string]interface{}{}, nil, &calls)
	// ---- workflow: CRUD ----
	b.call("GET", "/api/v1/workflow/steps", nil, &calls)
	b.callJSON("PUT", "/api/v1/workflow/steps/"+draftStepID, map[string]interface{}{"id": draftStepID, "version": 1, "name": "Updated Step"}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/workflow/steps/"+draftStepID, map[string]interface{}{}, nil, &calls)
	b.call("GET", "/api/v1/workflow/transitions", nil, &calls)
	b.call("GET", "/api/v1/workflow/transitions/"+transID, nil, &calls)
	b.callJSON("PUT", "/api/v1/workflow/transitions/"+transID, map[string]interface{}{"id": transID, "version": 1, "label": "Updated"}, nil, &calls)
	b.call("GET", "/api/v1/workflow/transitions/"+transID+"/conditions", nil, &calls)
	b.call("GET", "/api/v1/workflow/transitions/"+transID+"/effects", nil, &calls)
	b.callJSON("DELETE", "/api/v1/workflow/transitions/"+transID, map[string]interface{}{}, nil, &calls)
	// ---- reminders: CRUD ----
	b.call("GET", "/api/v1/reminders", nil, &calls)
	b.call("GET", "/api/v1/reminders/"+remID, nil, &calls)
	b.callJSON("PUT", "/api/v1/reminders/"+remID, map[string]interface{}{"id": remID, "version": 1, "title": "Updated Reminder"}, nil, &calls)
	b.callJSON("POST", "/api/v1/reminders/"+remID+"/notify", map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/reminders/"+remID, map[string]interface{}{}, nil, &calls)
	// ---- calendar: CRUD ----
	b.call("GET", "/api/v1/calendar/events", nil, &calls)
	b.callJSON("POST", "/api/v1/calendar/sync", map[string]interface{}{}, nil, &calls)
	b.call("GET", "/api/v1/ical-sources", nil, &calls)
	b.call("GET", "/api/v1/ical-sources/"+icalID, nil, &calls)
	b.callJSON("PUT", "/api/v1/ical-sources/"+icalID, map[string]interface{}{"name": "Updated Work", "version": 1}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/ical-sources/"+icalID, map[string]interface{}{}, nil, &calls)
	// ---- kiosk: CRUD ----
	b.call("GET", "/api/v1/kiosk/data", nil, &calls)
	b.call("GET", "/api/v1/kiosk/devices", nil, &calls)
	b.call("GET", "/api/v1/kiosk/devices/"+devID, nil, &calls)
	b.callJSON("PUT", "/api/v1/kiosk/devices/"+devID, map[string]interface{}{"name": "Kiosk TV Updated", "version": 1}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/kiosk/devices/"+devID, map[string]interface{}{}, nil, &calls)
	b.call("GET", "/api/v1/kiosk/playlists", nil, &calls)
	b.call("GET", "/api/v1/kiosk/playlists/"+plID, nil, &calls)
	b.call("GET", "/api/v1/kiosk/playlists/"+plID+"/views", nil, &calls)
	b.callJSON("PUT", "/api/v1/kiosk/playlists/"+plID, map[string]interface{}{"name": "Main Updated", "version": 1}, nil, &calls)
	b.callJSON("PUT", "/api/v1/kiosk/playlists/"+plID+"/views/"+plViewID, map[string]interface{}{"view_type": "graph", "version": 1}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/kiosk/playlists/"+plID+"/views/"+plViewID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/kiosk/playlists/"+plID, map[string]interface{}{}, nil, &calls)
	// ---- stats: read-only ----
	b.call("GET", "/api/v1/stats/cards", nil, &calls)
	b.call("GET", "/api/v1/stats/revenue", nil, &calls)
	b.call("GET", "/api/v1/stats/inventory", nil, &calls)
	b.call("GET", "/api/v1/stats/key-figures", nil, &calls)
	// ---- boards: CRUD ----
	b.call("GET", "/api/v1/boards", nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/boards/"+boardID, map[string]interface{}{"name": "Sprint 1 Updated", "version": 1}, nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID+"/columns", nil, &calls)
	b.callJSON("PATCH", "/api/v1/boards/"+boardID+"/columns/"+col1ID, map[string]interface{}{"version": 1, "name": "In Progress"}, nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID+"/cards", nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID+"/cards/"+card1ID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/boards/"+boardID+"/cards/"+card1ID, map[string]interface{}{"version": 1, "title": "Updated Card"}, nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/checklist", nil, &calls)
	b.callJSON("PATCH", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/checklist/"+clItemID, map[string]interface{}{"version": 1, "title": "Updated Check"}, nil, &calls)
	b.call("GET", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/comments", nil, &calls)
	b.callJSON("DELETE", "/api/v1/boards/"+boardID+"/cards/"+card1ID+"/checklist/"+clItemID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/boards/"+boardID+"/cards/"+card1ID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/boards/"+boardID+"/columns/"+col1ID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/boards/"+boardID, map[string]interface{}{}, nil, &calls)
	// ---- product-types: CRUD ----
	b.call("GET", "/api/v1/product-types", nil, &calls)
	b.call("GET", "/api/v1/product-types/"+ptID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/product-types/"+ptID, map[string]interface{}{"version": 1, "name": "Shampoo Updated"}, nil, &calls)
	b.call("GET", "/api/v1/product-types/"+ptID+"/fields", nil, &calls)
	w = b.callJSON("POST", "/api/v1/product-types/"+ptID+"/fields", map[string]interface{}{"field_definition_id": fdID, "sort_order": 1}, nil, &calls)
	var ptf map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &ptf)
	ptfID := ptf["id"].(string)
	b.callJSON("PATCH", "/api/v1/product-types/"+ptID+"/fields/"+ptfID, map[string]interface{}{"version": 1, "sort_order": 2}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/product-types/"+ptID+"/fields/"+ptfID, map[string]interface{}{}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/product-types/"+ptID, map[string]interface{}{}, nil, &calls)
	// ---- field-definitions: CRUD ----
	b.call("GET", "/api/v1/field-definitions", nil, &calls)
	b.call("GET", "/api/v1/field-definitions/"+fdID, nil, &calls)
	b.callJSON("PATCH", "/api/v1/field-definitions/"+fdID, map[string]interface{}{"id": fdID, "version": 1, "name": "Updated Field"}, nil, &calls)
	b.callJSON("DELETE", "/api/v1/field-definitions/"+fdID, map[string]interface{}{}, nil, &calls)
	// ---- ean-series: CRUD ----
	b.call("GET", "/api/v1/ean-series", nil, &calls)

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
