// Package regexrouter is a mux for Go that matches routes with regular
// expressions.
//
// # Patterns
//
// A pattern is a raw Go regular expression (see the regexp/syntax package),
// matched against the request path with FindStringSubmatch.
//
// Patterns are NOT anchored automatically. Because FindStringSubmatch matches
// anywhere in the path, an unanchored pattern matches any path that merely
// contains it: the pattern `/users` also matches /api/users-admin/list. Anchor
// patterns with ^ and $ to match the whole path (`^/users$`) unless a partial
// match is genuinely intended. Invalid patterns panic at registration; use
// ValidPattern to check dynamically-built patterns first.
//
// Routes are evaluated in registration order and the first pattern that matches
// wins, so register more specific patterns before broader ones.
//
// # Parameters
//
// Named capture groups become request parameters, read with URLParam:
//
//	m.Get(`^/users/(?P<id>[0-9]+)$`, func(w http.ResponseWriter, r *http.Request) {
//		id := regexrouter.URLParam(r, "id")
//		...
//	})
//
// # Sub-routers
//
// Route mounts a sub-Router. The optional "subroute" capture group (see
// SubrouteParam) designates the remaining path the sub-Router matches against;
// its sub-patterns are matched against that remainder:
//
//	m.Route(`^/api/(?P<subroute>.*)$`, func(r regexrouter.Router) {
//		r.Get(`^widgets$`, ...) // matches GET /api/widgets
//	})
package regexrouter

import (
	"net/http"
)

// Router consisting of the core routing methods used by chi's Mux,
// using only the standard net/http.
type Router interface {
	http.Handler

	// Use appends one or more middlewares onto the Router stack.
	Use(middlewares ...func(http.Handler) http.Handler)

	// With adds inline middlewares for an endpoint handler.
	//With(middlewares ...func(http.Handler) http.Handler) Router

	// Group adds a new inline-Router along the current routing
	// path, with a fresh middleware stack for the inline-Router.
	Group(fn func(r Router)) Router

	// Route mounts a sub-Router along a `pattern`` string. It is the way to
	// compose sub-Routers; use a `(?P<subroute>...)` capture group in the
	// pattern to delegate the remaining path to the sub-Router.
	Route(pattern string, fn func(r Router)) Router

	// Handle and HandleFunc adds routes for `pattern` that matches
	// all HTTP methods.
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h http.HandlerFunc)

	// Method and MethodFunc adds routes for `pattern` that matches
	// the `method` HTTP method.
	Method(method, pattern string, h http.Handler)
	MethodFunc(method, pattern string, h http.HandlerFunc)

	// HTTP-method routing along `pattern`
	Connect(pattern string, h http.HandlerFunc)
	Delete(pattern string, h http.HandlerFunc)
	Get(pattern string, h http.HandlerFunc)
	Head(pattern string, h http.HandlerFunc)
	Options(pattern string, h http.HandlerFunc)
	Patch(pattern string, h http.HandlerFunc)
	Post(pattern string, h http.HandlerFunc)
	Put(pattern string, h http.HandlerFunc)
	Trace(pattern string, h http.HandlerFunc)

	// NotFound defines a handler to respond whenever a route could
	// not be found.
	NotFound(h http.HandlerFunc)

	// MethodNotAllowed defines a handler to respond whenever a method is
	// not allowed.
	MethodNotAllowed(h http.HandlerFunc)
}

// Middlewares type is a slice of standard middleware handlers with methods
// to compose middleware chains and http.Handler's.
type Middlewares []func(http.Handler) http.Handler
