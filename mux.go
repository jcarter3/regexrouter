package regexrouter

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
)

var _ Router = &Mux{}

type Mux struct {
	// Custom method not allowed handler
	methodNotAllowedHandler http.HandlerFunc

	// A reference to the parent mux used by subrouters when mounting
	// to a parent mux
	parent *Mux

	// Custom route not found handler
	notFoundHandler http.HandlerFunc

	// The middleware stack
	middlewares []func(http.Handler) http.Handler

	// Controls the behaviour of middleware chain generation when a mux
	// is registered as an inline group inside another mux.
	inline bool

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

type Config struct {
	NotFoundHandler         http.HandlerFunc
	MethodNotAllowedHandler http.HandlerFunc
}

// New returns a newly initialized Mux object that implements the Router
// interface.
func New(cfg *Config) *Mux {
	mux := &Mux{
		routes: routes{
			rts: []route{},
		},
	}
	if cfg != nil {
		mux.notFoundHandler = cfg.NotFoundHandler
		mux.methodNotAllowedHandler = cfg.MethodNotAllowedHandler
	}
	return mux
}

func (mx *Mux) Use(middlewares ...func(http.Handler) http.Handler) {
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

// Route mounts a sub-Router along a `pattern“ string.
func (mx *Mux) Route(pattern string, fn func(Router)) Router {
	sr := &Mux{}
	fn(sr)

	// todo: find a way to make this a known type
	mx.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		requestPath := ""
		unnamed, ok := r.Context().Value("unnamed").([]string)
		if ok || len(unnamed) > 0 {
			requestPath = unnamed[len(unnamed)-1]
		}
		r = r.WithContext(context.WithValue(r.Context(), "requestpath", requestPath))
		sr.ServeHTTP(w, r)
	})
	return nil
}

// Mount mounts a sub-Router along a `pattern“ string.
func (mx *Mux) Mount(pattern string, handler http.Handler) {
	mx.Method("all", pattern, handler)
}

func (mx *Mux) Handle(pattern string, handler http.Handler) {
	mx.Method("all", pattern, handler)
}

func (mx *Mux) HandleFunc(pattern string, handler http.HandlerFunc) {
	mx.Method("all", pattern, handler)
}

func (mx *Mux) Method(method, pattern string, handler http.Handler) {
	handler = mx.chainHandler(handler)
	
	for _, rr := range mx.routes.rts {
		if rr.regex.String() == pattern {
			rr.methodhandler[method] = handler
			return
		}
	}

	r := route{
		regex:         regexp.MustCompile(pattern),
		methodhandler: map[string]http.Handler{method: handler},
	}

	if mx.parent != nil && mx.inline {
		mx.parent.routes.append(r)
	} else {
		mx.routes.append(r)
	}
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
	requestpath, ok := r.Context().Value("requestpath").(string)
	if ok {
		path = requestpath
	}

	for _, route := range mx.routes.rts {
		matches := route.regex.FindStringSubmatch(path)
		if len(matches) <= 0 {
			continue
		}
		handler, ok := route.methodhandler[r.Method]
		if !ok {
			handler, ok = route.methodhandler["all"]
			if !ok {
				mx.handleMethodNotAllowed(w, r)
				slog.Debug("method not allowed", "method", r.Method, "path", path)
				return
			}
		}

		varNames := route.varNames
		if len(route.regex.SubexpNames()) > 1 {
			varNames = route.regex.SubexpNames()[1:]
		}
		ctx := r.Context()
		var unnamed []string
		for i, match := range matches[1:] {
			if i > len(varNames)-1 || varNames[i] == "" {
				unnamed = append(unnamed, match)
				continue
			}
			ctx = context.WithValue(ctx, varNames[i], match)
		}
		if len(unnamed) > 0 {
			ctx = context.WithValue(ctx, "unnamed", unnamed)
		}
		// Store the matched route pattern for metrics/observability
		ctx = context.WithValue(ctx, "routePattern", route.regex.String())

		handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	mx.handleNotFound(w, r)
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
		mx.parent.methodNotAllowedHandler(w, r)
		return
	}
	defaultMethodNotAllowedHandler(w, r)
}

func defaultMethodNotAllowedHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	w.Write([]byte("not allowed"))
}
