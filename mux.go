package regexrouter

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var _ Router = &Mux{}

// methodAll is the internal wildcard method key used by Handle and HandleFunc
// to register a handler for every HTTP method. It is "*" rather than a word
// like "all" so it cannot be confused with, or shadowed by, a real HTTP method
// name (which Method normalizes to upper case).
const methodAll = "*"

// routePatternSeparator joins the patterns of nested sub-routers when building
// http.Request.Pattern, so the matched route reads top-down (for example
// "^/route1/(?P<subroute>.*)$ > ^foo$"). r.Pattern is a human-readable,
// low-cardinality label for observability that maps back to the registered
// pattern(s) that matched; it is intentionally not itself valid regex. A word
// separator is used rather than "," because commas occur inside patterns (e.g.
// the "{1,3}" quantifier), which made the old comma-joined form ambiguous.
const routePatternSeparator = " > "

// SubrouteParam is the name of the optional capture group in a Route pattern
// whose match becomes the path that the mounted sub-Router matches against. For
// example:
//
//	m.Route(`^/api/(?P<subroute>.*)$`, func(r Router) {
//		r.Get(`^widgets$`, ...) // matches GET /api/widgets
//	})
//
// When a Route pattern has no "subroute" group, its sub-Router matches against
// the empty string (useful when the sub-routes are all `^$`). The captured
// value is also readable as an ordinary parameter via URLParam(r, SubrouteParam).
const SubrouteParam = "subroute"

// contextKey is an unexported type used for the router's own context keys so
// they cannot collide with keys defined in other packages.
type contextKey int

const (
	// ctxKeyRequestPath carries the remaining path a sub-Router should match
	// against, set by Route before delegating to the sub-Router.
	ctxKeyRequestPath contextKey = iota
)

// paramKey namespaces user-defined regex capture-group names stored in the
// request context, keeping them from colliding with the router's internal
// keys or with context keys from other packages.
type paramKey string

// URLParam returns the value of the named regex capture group for the current
// request, or "" if no such group matched.
func URLParam(r *http.Request, name string) string {
	return URLParamFromCtx(r.Context(), name)
}

// URLParamFromCtx returns the value of the named regex capture group stored in
// ctx, or "" if no such group matched.
func URLParamFromCtx(ctx context.Context, name string) string {
	v, _ := ctx.Value(paramKey(name)).(string)
	return v
}

type Mux struct {
	// Custom method not allowed handler
	methodNotAllowedHandler http.HandlerFunc

	// A reference to the parent mux used by subrouters when mounting
	// to a parent mux
	parent *Mux

	// Custom route not found handler
	notFoundHandler http.HandlerFunc

	// Debug logger; nil means fall back to the parent's, then a no-op. Set via
	// WithLogger. Resolved through log().
	logger Logger

	// The middleware stack
	middlewares []func(http.Handler) http.Handler

	// Controls the behaviour of middleware chain generation when a mux
	// is registered as an inline group inside another mux.
	inline bool

	// Set once any route has been registered through this mux (or, for an
	// inline mux, through the parent it appends to). Used to reject Use()
	// calls made after routes, whose middleware would otherwise be dropped.
	hasRoutes bool

	routes routes
}

type routes struct {
	rts []route
}

func (r *routes) append(rt route) {
	r.rts = append(r.rts, rt)
}

type route struct {
	regex         *regexp.Regexp
	methodhandler map[string]http.Handler
	varNames      []string
}

// Logger is the minimal logging surface regexrouter uses. *slog.Logger
// satisfies it directly, so New(WithLogger(slog.Default())) works without an
// adapter; other loggers need only a small shim.
type Logger interface {
	Debug(msg string, args ...any)
}

// noopLogger is the default logger: a library should not write to the global
// logger unless the caller asks it to.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}

// Option configures a Mux at construction time. Pass options to New.
type Option func(*Mux)

// WithNotFoundHandler sets the handler invoked when no route matches the
// request path.
func WithNotFoundHandler(h http.HandlerFunc) Option {
	return func(mx *Mux) { mx.notFoundHandler = h }
}

// WithMethodNotAllowedHandler sets the handler invoked when a route matches the
// request path but not its method.
func WithMethodNotAllowedHandler(h http.HandlerFunc) Option {
	return func(mx *Mux) { mx.methodNotAllowedHandler = h }
}

// WithLogger sets the debug logger. By default the router logs nothing.
func WithLogger(l Logger) Option {
	return func(mx *Mux) { mx.logger = l }
}

// New returns a newly initialized Mux that implements the Router interface,
// configured by the given options. Call New() for defaults, or pass options
// such as WithNotFoundHandler to customize behavior.
func New(opts ...Option) *Mux {
	mux := &Mux{
		routes: routes{
			rts: []route{},
		},
	}
	for _, opt := range opts {
		opt(mux)
	}
	return mux
}

// ValidPattern reports whether pattern is a valid route pattern, i.e. a
// compilable regular expression, returning the compilation error otherwise.
// The registration methods (Get, Method, Route, ...) panic on an invalid
// pattern, so use ValidPattern to check dynamically-built patterns before
// registering them.
func ValidPattern(pattern string) error {
	_, err := regexp.Compile(pattern)
	return err
}

func (mx *Mux) Use(middlewares ...func(http.Handler) http.Handler) {
	// Middleware chains are baked into each handler at registration time, so a
	// middleware added after a route would silently never run. Fail loudly
	// instead of dropping it.
	if mx.hasRoutes {
		panic("regexrouter: all middlewares must be registered before routes")
	}
	mx.middlewares = append(mx.middlewares, middlewares...)
}

func (mx *Mux) With(middlewares ...func(http.Handler) http.Handler) Router {
	return &Mux{
		middlewares: middlewares,
		parent:      mx,
		inline:      true,
	}
}

func (mx *Mux) Group(fn func(r Router)) Router {
	im := mx.With()
	if fn != nil {
		fn(im)
	}
	return im
}

// Route mounts a sub-Router along a `pattern“ string and returns it. The
// returned Router stays live: routes registered on it after Route returns are
// still matched. Registering routes inside fn and then calling Use on the
// returned Router will panic (see Use); add middleware inside fn instead.
func (mx *Mux) Route(pattern string, fn func(Router)) Router {
	if fn == nil {
		panic("regexrouter: Route requires a non-nil configuration func")
	}
	// Wire the parent (but leave inline false) so the sub-Router falls back to
	// the parent's NotFound/MethodNotAllowed handlers when it has none of its
	// own. inline stays false so the sub-Router keeps its own route table and
	// its middleware is not re-chained through the parent (parent middleware
	// already wraps the entry point registered by HandleFunc below).
	sr := &Mux{parent: mx}
	fn(sr)

	// When the pattern has no "subroute" capture group, the sub-Router always
	// matches against the empty remainder, so any sub-route that cannot match
	// "" is unreachable. That is almost always a forgotten (?P<subroute>...)
	// group, so fail loudly at registration instead of 404-ing at request time.
	if !hasSubrouteGroup(pattern) {
		for _, rt := range sr.routes.rts {
			if !rt.regex.MatchString("") {
				panic(fmt.Sprintf("regexrouter: Route pattern %q has no (?P<%s>...) capture group, "+
					"so its sub-Router only matches the empty remainder, but sub-route %q cannot "+
					"match it and is unreachable", pattern, SubrouteParam, rt.regex.String()))
			}
		}
	}

	mx.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// The value captured by the "subroute" group (if present) is the path
		// the sub-Router matches against; without it the sub-Router sees "".
		requestPath := URLParamFromCtx(r.Context(), SubrouteParam)
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyRequestPath, requestPath))
		sr.ServeHTTP(w, r)
	})
	return sr
}

func (mx *Mux) Handle(pattern string, handler http.Handler) {
	mx.Method(methodAll, pattern, handler)
}

func (mx *Mux) HandleFunc(pattern string, handler http.HandlerFunc) {
	mx.Method(methodAll, pattern, handler)
}

func (mx *Mux) Method(method, pattern string, handler http.Handler) {
	// Normalize the method so registrations are case-insensitive and match the
	// upper-case r.Method values used at dispatch time. The wildcard sentinel
	// is upper-case-stable, so this is safe for it too.
	if method != methodAll {
		method = strings.ToUpper(method)
	}
	handler = mx.chainHandler(handler)
	mx.hasRoutes = true

	for _, rr := range mx.routes.rts {
		if rr.regex.String() == pattern {
			rr.methodhandler[method] = handler
			return
		}
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(fmt.Sprintf("regexrouter: invalid route pattern %q: %v", pattern, err))
	}
	r := route{
		regex:         re,
		methodhandler: map[string]http.Handler{method: handler},
		varNames:      captureNames(re),
	}

	if mx.parent != nil && mx.inline {
		mx.parent.routes.append(r)
		mx.parent.hasRoutes = true
	} else {
		mx.routes.append(r)
	}
}

// hasSubrouteGroup reports whether pattern contains a capture group named
// SubrouteParam. pattern is assumed valid; an invalid pattern is treated as
// having no such group (its compilation error surfaces at registration).
func hasSubrouteGroup(pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	for _, n := range re.SubexpNames() {
		if n == SubrouteParam {
			return true
		}
	}
	return false
}

// captureNames returns the names of a compiled pattern's capture groups (in
// order, excluding the whole-match group at index 0). Unnamed groups yield "".
func captureNames(re *regexp.Regexp) []string {
	names := re.SubexpNames()
	if len(names) <= 1 {
		return nil
	}
	return names[1:]
}

func (mx *Mux) MethodFunc(method, pattern string, handler http.HandlerFunc) {
	mx.Method(method, pattern, handler)
}

func (mx *Mux) Connect(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodConnect, pattern, handler)
}

func (mx *Mux) Delete(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodDelete, pattern, handler)
}

func (mx *Mux) Get(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodGet, pattern, handler)
}

func (mx *Mux) Head(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodHead, pattern, handler)
}

func (mx *Mux) Options(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodOptions, pattern, handler)
}

func (mx *Mux) Patch(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodPatch, pattern, handler)
}

func (mx *Mux) Post(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodPost, pattern, handler)
}

func (mx *Mux) Put(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodPut, pattern, handler)
}

func (mx *Mux) Trace(pattern string, handler http.HandlerFunc) {
	mx.MethodFunc(http.MethodTrace, pattern, handler)
}

func (mx *Mux) NotFound(handler http.HandlerFunc) {
	mx.notFoundHandler = handler
}

func (mx *Mux) MethodNotAllowed(handler http.HandlerFunc) {
	mx.methodNotAllowedHandler = handler
}

func (mx *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if requestpath, ok := r.Context().Value(ctxKeyRequestPath).(string); ok {
		path = requestpath
	}

	// pathMatched tracks whether any route matched the path but not the
	// method, so we can distinguish 405 (Method Not Allowed) from 404 (Not
	// Found) only after considering every overlapping pattern.
	pathMatched := false

	for _, route := range mx.routes.rts {
		matches := route.regex.FindStringSubmatch(path)
		if len(matches) <= 0 {
			continue
		}
		handler, ok := route.methodhandler[r.Method]
		if !ok {
			handler, ok = route.methodhandler[methodAll]
		}
		if !ok {
			// This pattern matched the path but has no handler for the
			// method. Keep scanning: another overlapping pattern may.
			pathMatched = true
			continue
		}

		ctx := r.Context()
		for i, match := range matches[1:] {
			if i > len(route.varNames)-1 || route.varNames[i] == "" {
				// Unnamed capture group: not exposed as a parameter.
				continue
			}
			ctx = context.WithValue(ctx, paramKey(route.varNames[i]), match)
		}
		if r.Pattern == "" {
			r.Pattern = route.regex.String()
		} else {
			r.Pattern = r.Pattern + routePatternSeparator + route.regex.String()
		}
		handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	if pathMatched {
		mx.handleMethodNotAllowed(w, r)
		mx.log().Debug("method not allowed", "method", r.Method, "path", path)
		return
	}
	mx.handleNotFound(w, r)
}

// log resolves the logger for this mux: its own if set, otherwise the parent's,
// falling back to a no-op. This mirrors the NotFound/MethodNotAllowed fallback
// so sub-Routers inherit the logger configured on the root.
func (mx *Mux) log() Logger {
	if mx.logger != nil {
		return mx.logger
	}
	if mx.parent != nil {
		return mx.parent.log()
	}
	return noopLogger{}
}

func (mx *Mux) chainHandler(handler http.Handler) http.Handler {
	for i := len(mx.middlewares) - 1; i >= 0; i-- {
		handler = mx.middlewares[i](handler)
	}
	if mx.parent != nil && mx.inline {
		handler = mx.parent.chainHandler(handler)
	}
	return handler
}

func (mx *Mux) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if mx.notFoundHandler != nil {
		mx.notFoundHandler(w, r)
		return
	}
	if mx.parent != nil {
		mx.parent.handleNotFound(w, r)
		return
	}
	defaultNotFoundHandler(w, r)
}

func defaultNotFoundHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("not found"))
}

func (mx *Mux) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if mx.methodNotAllowedHandler != nil {
		mx.methodNotAllowedHandler(w, r)
		return
	}
	if mx.parent != nil {
		mx.parent.handleMethodNotAllowed(w, r)
		return
	}
	defaultMethodNotAllowedHandler(w, r)
}

func defaultMethodNotAllowedHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	w.Write([]byte("not allowed"))
}
