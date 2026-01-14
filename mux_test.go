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
	m := New(nil)

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
		w.Write([]byte(fmt.Sprintf("%s %s", r.Context().Value("var1"), r.Context().Value("var2"))))
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
	m := New(nil)

	m.Get(`^/$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	m.Route(`^/route1/(.*)$`, func(r Router) {
		r.Get("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("ok1"))
		})
		r.Get(`^foo$`, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("foo1"))
		})
	})
	m.Route(`^/route2/(.*)$`, func(r Router) {
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
	m := New(nil)

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
	m := New(nil)

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
	m := New(nil)

	m.Route(`^/v2/(?P<name>[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*)/manifests/(?P<reference>.*)$`, func(rr Router) {
		rr.Head("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		})
		rr.Get("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest get: %s %s", r.Context().Value("name"), r.Context().Value("reference"))
		})
		rr.Put("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest put: %s %s", r.Context().Value("name"), r.Context().Value("reference"))
		})
		rr.Delete("^$", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			fmt.Fprintf(w, "manifest delete: %s %s", r.Context().Value("name"), r.Context().Value("reference"))
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
	m := New(nil)

	m.Get(`^/$`, returnPattern())
	m.Get(`^/path$`, returnPattern())
	m.Route(`^/route1/(.*)$`, func(r Router) {
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
			expectedBody:   `^/route1/(.*)$,^foo$`,
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
