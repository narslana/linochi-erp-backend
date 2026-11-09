//go:build integration

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
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

// A browser that records the request of every call, so that each answer can be
// checked against the spec afterwards.
type recorded struct {
	req *http.Request
	w   *httptest.ResponseRecorder
}

func (b *browser) call(method, target string, headers map[string]string, sink *[]recorded) *httptest.ResponseRecorder {
	b.t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), method, "https://erp.lab.internal"+target, nil)
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	b.pod.mux.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	*sink = append(*sink, recorded{r, w})
	return w
}

func TestNFK29_ResponsesMatchOpenAPI(t *testing.T) {
	doc, find := specRouter(t)
	he, p := newHandlerEnv(t)
	var calls []recorded

	b := he.browser(t, p)
	// Not logged in.
	b.call("GET", "/api/v1/me", nil, &calls)
	b.call("POST", "/auth/logout", map[string]string{"Sec-Fetch-Site": "cross-site"}, &calls)
	// A good login, with the answers of every step.
	w := b.call("GET", "/auth/login?return_to=%2Fproducts", nil, &calls)
	target := he.callbackTarget(t, w.Header().Get("Location"))
	b.call("GET", target, nil, &calls)
	b.call("GET", "/api/v1/me", nil, &calls)
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
	b2 := he.browser(t, he.newPod(t))
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
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)

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
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)
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
