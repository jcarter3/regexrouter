package regexrouter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testCase struct {
	name           string
	path           string
	method         string
	body           io.Reader
	expectedStatus int
	expectedBody   string
}

func TestMuxBasic(t *testing.T) {
	m := New()

	m.Get(`^/$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	m.Get(`^/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("get path"))
	})
	m.Post(`^/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("post path"))
	})
	m.Patch(`^/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("patch path"))
	})
	m.Get(`/(?P<var1>.*)/(?P<var2>.*)/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%s %s", URLParam(r, "var1"), URLParam(r, "var2"))))
	})
	m.HandleFunc(`^/allmethods$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("all methods"))
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "get root",
			path:           "/",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "ok",
		},
		{
			name:           "not found",
			path:           "/notfound",
			method:         "GET",
			expectedStatus: 404,
			expectedBody:   "not found",
		}, {
			name:           "get path",
			path:           "/path",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "get path",
		}, {
			name:           "post path",
			path:           "/path",
			method:         "POST",
			expectedStatus: 200,
			expectedBody:   "post path",
		}, {
			name:           "patch path",
			path:           "/path",
			method:         "PATCH",
			expectedStatus: 200,
			expectedBody:   "patch path",
		}, {
			name:           "delete path",
			path:           "/path",
			method:         "DELETE",
			expectedStatus: 405,
			expectedBody:   "not allowed",
		}, {
			name:           "get path with var1 and var2",
			path:           "/foo/bar/path",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "foo bar",
		}, {
			name:           "all methods",
			path:           "/allmethods",
			method:         "OPTIONS",
			expectedStatus: 200,
			expectedBody:   "all methods",
		},
	}

	runTestCases(t, ts, testCases)
}

func TestSubRouters(t *testing.T) {
	m := New()

	m.Get(`^/$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	m.Route(`^/route1/(?P<subroute>.*)$`, func(r Router) {
		r.Get("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("ok1"))
		})
		r.Get(`^foo$`, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("foo1"))
		})
	})
	m.Route(`^/route2/(?P<subroute>.*)$`, func(r Router) {
		r.Get("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("ok2"))
		})
		r.Get(`^foo$`, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("foo2"))
		})
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "get root",
			path:           "/",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "ok",
		}, {
			name:           "get sub1 foo1",
			path:           "/route1/foo",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "foo1",
		}, {
			name:           "get sub2 foo1",
			path:           "/route2/foo",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "foo2",
		},
	}

	runTestCases(t, ts, testCases)
}

func TestMiddlewares(t *testing.T) {
	m := New()

	m.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := r.Context().Value("middlewares").([]string)
			if !ok {
				v = []string{}
			}
			v = append(v, "1")
			r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
			next.ServeHTTP(w, r)
		})
	})
	m.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := r.Context().Value("middlewares").([]string)
			if !ok {
				t.Fatalf("failed to get middlewares from context")
			}
			v = append(v, "2")
			r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
			next.ServeHTTP(w, r)
		})
	})

	m.Get(`^\/$`, returnMWs(t))

	m.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := r.Context().Value("middlewares").([]string)
			if !ok {
				t.Fatalf("failed to get middlewares from context")
			}
			v = append(v, "a")
			r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
			next.ServeHTTP(w, r)
		})
	}).Get(`^\/foo$`, returnMWs(t))

	m.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := r.Context().Value("middlewares").([]string)
			if !ok {
				t.Fatalf("failed to get middlewares from context")
			}
			v = append(v, "b")
			r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
			next.ServeHTTP(w, r)
		})
	}).Get(`^/bar$`, returnMWs(t))

	m.Get(`^/baz$`, returnMWs(t))

	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "get root",
			path:           "/",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 2",
		}, {
			name:           "get foo",
			path:           "/foo",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 2 a",
		}, {
			name:           "get bar",
			path:           "/bar",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 2 b",
		}, {
			name:           "get baz",
			path:           "/baz",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 2",
		},
	}

	runTestCases(t, ts, testCases)
}

func TestGrouping(t *testing.T) {
	m := New()

	m.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := r.Context().Value("middlewares").([]string)
			if !ok {
				v = []string{}
			}
			v = append(v, "1")
			r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
			next.ServeHTTP(w, r)
		})
	})

	m.Group(func(r Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				v, ok := r.Context().Value("middlewares").([]string)
				if !ok {
					t.Fatalf("failed to get middlewares from context")
				}
				v = append(v, "a")
				r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
				next.ServeHTTP(w, r)
			})
		})
		r.Get(`^\/foo$`, returnMWs(t))
	})

	m.Group(func(r Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				v, ok := r.Context().Value("middlewares").([]string)
				if !ok {
					t.Fatalf("failed to get middlewares from context")
				}
				v = append(v, "b")
				r = r.WithContext(context.WithValue(r.Context(), "middlewares", v))
				next.ServeHTTP(w, r)
			})
		})
		r.Get(`^/bar$`, returnMWs(t))
	})
	m.Get(`^/$`, returnMWs(t))
	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "get root",
			path:           "/",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1",
		}, {
			name:           "get foo",
			path:           "/foo",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 a",
		}, {
			name:           "get bar",
			path:           "/bar",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "1 b",
		},
	}

	runTestCases(t, ts, testCases)
}

func TestOCIDistRouting(t *testing.T) {
	m := New()

	m.Route(`^/v2/(?P<name>[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*)/manifests/(?P<reference>.*)$`, func(rr Router) {
		rr.Head("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		})
		rr.Get("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest get: %s %s", URLParam(r, "name"), URLParam(r, "reference"))
		})
		rr.Put("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest put: %s %s", URLParam(r, "name"), URLParam(r, "reference"))
		})
		rr.Delete("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest delete: %s %s", URLParam(r, "name"), URLParam(r, "reference"))
		})
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "head",
			path:           "/v2/foo/bar/baz/manifests/tag",
			method:         "HEAD",
			expectedStatus: 200,
			expectedBody:   "",
		}, {
			name:           "get",
			path:           "/v2/foo/manifests/tag",
			method:         "GET",
			expectedStatus: 200,
			expectedBody:   "manifest get: foo tag",
		}, {
			name:           "put",
			path:           "/v2/foo/bar/manifests/tag",
			method:         "PUT",
			expectedStatus: 200,
			expectedBody:   "manifest put: foo/bar tag",
		}, {
			name:           "delete",
			path:           "/v2/foo/bar/baz/manifests/tag",
			method:         "DELETE",
			expectedStatus: 200,
			expectedBody:   "manifest delete: foo/bar/baz tag",
		},
	}

	runTestCases(t, ts, testCases)
}

func returnMWs(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, ok := r.Context().Value("middlewares").([]string)
		if !ok {
			t.Fatalf("failed to get middlewares from context")
		}
		w.WriteHeader(200)
		w.Write([]byte(strings.Join(v, " ")))
	}
}

func TestRequestPattern(t *testing.T) {
	m := New()

	m.Get(`^/$`, returnPattern())
	m.Get(`^/path$`, returnPattern())
	m.Route(`^/route1/(?P<subroute>.*)$`, func(r Router) {
		r.Get("^$", returnPattern())
		r.Get(`^foo$`, returnPattern())
	})

	testCases := []testCase{
		{
			name:           "get root",
			path:           "/",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   `^/$`,
		}, {
			name:           "get path",
			path:           "/path",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   `^/path$`,
		}, {
			name:           "get route1 foo",
			path:           "/route1/foo",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   `^/route1/(?P<subroute>.*)$ > ^foo$`,
		},
	}
	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, testCases)
}

func returnPattern() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Pattern))
	}
}

// TestMethodDispatchAcrossOverlappingPatterns guards against the 405
// short-circuit: a request whose method is served by a later, overlapping
// pattern must be dispatched rather than rejected by the first pattern that
// only matched the path.
func TestMethodDispatchAcrossOverlappingPatterns(t *testing.T) {
	m := New()
	m.Get(`^/x$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("get"))
	})
	m.Post(`^/x.*$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("post"))
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	testCases := []testCase{
		{
			name:           "get matches first pattern",
			path:           "/x",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "get",
		}, {
			name:           "post falls through to overlapping pattern",
			path:           "/x",
			method:         http.MethodPost,
			expectedStatus: http.StatusOK,
			expectedBody:   "post",
		}, {
			name:           "unhandled method still yields 405",
			path:           "/x",
			method:         http.MethodDelete,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "not allowed",
		},
	}

	runTestCases(t, ts, testCases)
}

// TestURLParamNamespacing verifies that capture-group values are retrieved via
// URLParam and that a group named like an internal key does not corrupt state.
func TestURLParamNamespacing(t *testing.T) {
	m := New()
	m.Get(`^/a/(?P<requestpath>.*)$`, func(w http.ResponseWriter, r *http.Request) {
		// Reading the raw internal key must not leak the router's bookkeeping,
		// and the user's value is available via URLParam.
		if raw := r.Context().Value("requestpath"); raw != nil {
			t.Fatalf("internal string key leaked into user namespace: %v", raw)
		}
		w.Write([]byte(URLParam(r, "requestpath")))
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, []testCase{{
		name:           "named group retrieved via URLParam",
		path:           "/a/hello",
		method:         http.MethodGet,
		expectedStatus: http.StatusOK,
		expectedBody:   "hello",
	}})
}

// TestUseAfterRoutePanics verifies that registering middleware after a route
// fails loudly instead of silently dropping the middleware.
func TestUseAfterRoutePanics(t *testing.T) {
	m := New()
	m.Get(`^/$`, func(w http.ResponseWriter, r *http.Request) {})

	defer func() {
		if recover() == nil {
			t.Fatal("expected Use() after a route registration to panic")
		}
	}()
	m.Use(func(next http.Handler) http.Handler { return next })
}

// TestRouteReturnsLiveRouter verifies Route returns a usable sub-Router (not
// nil) and that routes registered on it after Route returns are still matched.
func TestRouteReturnsLiveRouter(t *testing.T) {
	m := New()
	sub := m.Route(`^/api/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^inside$`, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("inside"))
		})
	})
	if sub == nil {
		t.Fatal("Route returned nil; expected a Router")
	}
	// Register a route on the returned sub-Router after Route has returned.
	sub.Get(`^after$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("after"))
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, []testCase{
		{
			name:           "route registered inside fn",
			path:           "/api/inside",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "inside",
		}, {
			name:           "route registered on returned router",
			path:           "/api/after",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "after",
		},
	})
}

// TestRouteNilFuncPanics verifies Route fails loudly with a clear message when
// given a nil configuration func.
func TestRouteNilFuncPanics(t *testing.T) {
	m := New()
	defer func() {
		if recover() == nil {
			t.Fatal("expected Route with nil fn to panic")
		}
	}()
	m.Route(`^/x$`, nil)
}

// TestSubRouterInheritsNotFound verifies a sub-Router with no NotFound handler
// of its own falls back to the parent's custom handler (finding #7).
func TestSubRouterInheritsNotFound(t *testing.T) {
	m := New(WithNotFoundHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("CUSTOM-404"))
	}))
	m.Route(`^/r/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^known$`, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, []testCase{
		{
			name:           "matched sub-route",
			path:           "/r/known",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		}, {
			name:           "unmatched sub-route inherits parent NotFound",
			path:           "/r/unknown",
			method:         http.MethodGet,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "CUSTOM-404",
		},
	})
}

// TestSubRouterMethodNotAllowed verifies that a method-not-allowed inside a
// sub-Router walks up to the parent instead of dereferencing a nil handler
// field (finding #6), both with and without a custom parent handler.
func TestSubRouterMethodNotAllowed(t *testing.T) {
	// No custom handlers anywhere: previously this path dereferenced the
	// parent's nil methodNotAllowedHandler and panicked.
	def := New()
	def.Route(`^/r/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^known$`, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})
	})
	tsDef := httptest.NewServer(def)
	defer tsDef.Close()
	runTestCases(t, tsDef, []testCase{{
		name:           "default 405 via parent walk (no panic)",
		path:           "/r/known",
		method:         http.MethodPost,
		expectedStatus: http.StatusMethodNotAllowed,
		expectedBody:   "not allowed",
	}})

	// Custom parent handler is inherited by the sub-Router.
	custom := New(WithMethodNotAllowedHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("CUSTOM-405"))
	}))
	custom.Route(`^/r/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^known$`, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})
	})
	tsCustom := httptest.NewServer(custom)
	defer tsCustom.Close()
	runTestCases(t, tsCustom, []testCase{{
		name:           "custom 405 inherited from parent",
		path:           "/r/known",
		method:         http.MethodPost,
		expectedStatus: http.StatusMethodNotAllowed,
		expectedBody:   "CUSTOM-405",
	}})
}

// TestMethodCaseNormalization verifies that Method normalizes the HTTP method
// so a lower/mixed-case registration still matches the upper-case method sent
// on the request (finding #10).
func TestMethodCaseNormalization(t *testing.T) {
	m := New()
	m.Method("get", `^/lower$`, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("lower"))
	}))
	m.MethodFunc("PoSt", `^/mixed$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("mixed"))
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, []testCase{
		{
			name:           "lower-case registration matches GET",
			path:           "/lower",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "lower",
		}, {
			name:           "mixed-case registration matches POST",
			path:           "/mixed",
			method:         http.MethodPost,
			expectedStatus: http.StatusOK,
			expectedBody:   "mixed",
		},
	})
}

// TestNewOptions verifies New works with no options and applies functional
// options such as WithNotFoundHandler / WithMethodNotAllowedHandler.
func TestNewOptions(t *testing.T) {
	if New() == nil {
		t.Fatal("New() must return a usable Mux")
	}

	m := New(
		WithNotFoundHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("CUSTOM-404"))
		}),
		WithMethodNotAllowedHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("CUSTOM-405"))
		}),
	)
	m.Get(`^/exists$`, func(w http.ResponseWriter, r *http.Request) {})
	ts := httptest.NewServer(m)
	defer ts.Close()
	runTestCases(t, ts, []testCase{
		{
			name:           "WithNotFoundHandler applied",
			path:           "/missing",
			method:         http.MethodGet,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "CUSTOM-404",
		}, {
			name:           "WithMethodNotAllowedHandler applied",
			path:           "/exists",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "CUSTOM-405",
		},
	})
}

// TestValidPatternAndInvalidPatternPanic verifies dynamically-built patterns can
// be validated up front, and that registering an invalid pattern panics with an
// actionable message (finding #17).
func TestValidPatternAndInvalidPatternPanic(t *testing.T) {
	if err := ValidPattern(`^/ok/(.*)$`); err != nil {
		t.Fatalf("valid pattern reported error: %v", err)
	}
	if ValidPattern(`^/(`) == nil {
		t.Fatal("invalid pattern reported no error")
	}

	m := New()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected registering an invalid pattern to panic")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "invalid route pattern") {
			t.Fatalf("panic message not actionable: %v", r)
		}
	}()
	m.Get(`^/(`, func(w http.ResponseWriter, r *http.Request) {})
}

// TestSubrouteParam documents the explicit "subroute" capture-group contract:
// its match is the path the sub-Router sees, it is also readable via URLParam,
// and a Route with no such group makes the sub-Router match against "".
func TestSubrouteParam(t *testing.T) {
	m := New()
	// Explicit remainder via the subroute group.
	m.Route(`^/api/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^widgets$`, func(w http.ResponseWriter, r *http.Request) {
			// The remainder is also available as an ordinary parameter.
			fmt.Fprintf(w, "widgets sub=%q", URLParam(r, SubrouteParam))
		})
	})
	// No subroute group: sub-Router matches against "".
	m.Route(`^/health/(?P<name>[a-z]+)$`, func(r Router) {
		r.Get(`^$`, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "health %s", URLParam(r, "name"))
		})
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	runTestCases(t, ts, []testCase{
		{
			name:           "subroute remainder routes and is readable",
			path:           "/api/widgets",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   `widgets sub="widgets"`,
		}, {
			name:           "no subroute group matches empty remainder",
			path:           "/health/db",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "health db",
		},
	})
}

// TestRouteMissingSubrouteGroupPanics verifies that a Route whose sub-routes
// expect a non-empty remainder but whose pattern has no "subroute" group fails
// loudly at registration rather than silently 404-ing.
func TestRouteMissingSubrouteGroupPanics(t *testing.T) {
	m := New()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing subroute group")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, SubrouteParam) {
			t.Fatalf("panic message not actionable: %v", r)
		}
	}()
	// Unnamed group is no longer treated as the remainder, so ^widgets$ can
	// never match the (empty) remainder — this is the classic mistake.
	m.Route(`^/api/(.*)$`, func(r Router) {
		r.Get(`^widgets$`, func(w http.ResponseWriter, r *http.Request) {})
	})
}

// TestRouteNoSubrouteGroupEmptyRemainderOK verifies the legitimate no-group
// case: when every sub-route matches the empty remainder, registration succeeds.
func TestRouteNoSubrouteGroupEmptyRemainderOK(t *testing.T) {
	m := New()
	m.Route(`^/health/(?P<name>[a-z]+)$`, func(r Router) {
		r.Get(`^$`, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})
	})
	ts := httptest.NewServer(m)
	defer ts.Close()
	runTestCases(t, ts, []testCase{{
		name:           "empty-remainder sub-route reachable",
		path:           "/health/db",
		method:         http.MethodGet,
		expectedStatus: http.StatusOK,
		expectedBody:   "ok",
	}})
}

// captureLogger records Debug messages for assertions.
type captureLogger struct{ msgs []string }

func (c *captureLogger) Debug(msg string, _ ...any) { c.msgs = append(c.msgs, msg) }

// TestWithLogger verifies the debug logger is used on the 405 path and that a
// sub-Router inherits the logger configured on the root (via the parent walk),
// while by default nothing is logged.
func TestWithLogger(t *testing.T) {
	logger := &captureLogger{}
	m := New(WithLogger(logger))
	m.Route(`^/r/(?P<subroute>.*)$`, func(r Router) {
		r.Get(`^known$`, func(w http.ResponseWriter, r *http.Request) {})
	})

	ts := httptest.NewServer(m)
	defer ts.Close()

	// GET matches -> no log. POST matches path but not method -> 405 -> logged
	// from within the sub-Router, which has no logger of its own.
	testRequest(t, ts, http.MethodGet, "/r/known", nil)
	if len(logger.msgs) != 0 {
		t.Fatalf("expected no logs on a successful match, got %v", logger.msgs)
	}
	testRequest(t, ts, http.MethodPost, "/r/known", nil)
	if len(logger.msgs) != 1 || logger.msgs[0] != "method not allowed" {
		t.Fatalf("expected one 'method not allowed' log inherited by sub-Router, got %v", logger.msgs)
	}
}

// TestDefaultLoggerIsNoop verifies the default logger neither logs nor panics
// on the 405 path when no logger is configured.
func TestDefaultLoggerIsNoop(t *testing.T) {
	m := New()
	m.Get(`^/x$`, func(w http.ResponseWriter, r *http.Request) {})
	ts := httptest.NewServer(m)
	defer ts.Close()
	runTestCases(t, ts, []testCase{{
		name:           "405 with default no-op logger does not panic",
		path:           "/x",
		method:         http.MethodDelete,
		expectedStatus: http.StatusMethodNotAllowed,
		expectedBody:   "not allowed",
	}})
}

func runTestCases(t *testing.T, ts *httptest.Server, testCases []testCase) {
	for _, tc := range testCases {
		resp, body := testRequest(t, ts, tc.method, tc.path, tc.body)
		if resp.StatusCode != tc.expectedStatus {
			t.Fatalf("test case '%s' failed, expected status %d, got %d", tc.name, tc.expectedStatus, resp.StatusCode)
		}
		if body != tc.expectedBody {
			t.Fatalf("test case '%s' failed, expected body '%s', got '%s'", tc.name, tc.expectedBody, body)
		}
	}
}

func testRequest(t *testing.T, ts *httptest.Server, method, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}
	defer resp.Body.Close()

	return resp, string(respBody)
}
