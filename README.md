`regexrouter` is a Go http router that uses regular expressions to match routes. You should almost never
use it. If you do, you're probably doing something wrong. But sometimes... you just do need it.
`regexrouter` was written out of such a need. It was modeled after the `chi` router to provide a familiar
interface.

## Install

```shell
go get github.com/jcarter3/regexrouter
```

## Features

* Works!
* Familiar API!
* 100% compatible with net/http

## Examples

**As easy as:**

```go
package main

import (
	"fmt"
	"net/http"
    
	"github.com/jscarter3/regexrouter"
)

func main() {
	r := regexrouter.New()
	r.Get(`^\/$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	r.Get(`^\/(?P<var1>.*)\/(?P<var2>.*)\/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%s %s", regexrouter.URLParam(r, "var1"), regexrouter.URLParam(r, "var2"))))
	})
	http.ListenAndServe(":3000", r)
}
```

## Patterns

Route patterns are raw [Go regular expressions](https://pkg.go.dev/regexp/syntax),
matched against the request path.

* **Patterns are not anchored for you.** Matching uses `FindStringSubmatch`, which
  matches *anywhere* in the path, so an unanchored pattern matches any path that merely
  contains it — `/users` also matches `/api/users-admin/list`. Anchor with `^` and `$`
  (`^/users$`) to match the whole path unless a partial match is genuinely intended.
* **Order matters.** Routes are evaluated in registration order and the first matching
  pattern wins. Register specific patterns before broader ones.
* **Invalid patterns panic at registration.** Use `regexrouter.ValidPattern(pattern)` to
  validate dynamically-built patterns first.

## Parameters

Named capture groups become request parameters, read with `URLParam`:

```go
r.Get(`^/users/(?P<id>[0-9]+)$`, func(w http.ResponseWriter, r *http.Request) {
	id := regexrouter.URLParam(r, "id")
	w.Write([]byte(id))
})
```

## Sub-routers

`Route` mounts a sub-router. Use a capture group named `subroute` for the remaining path
the sub-router should match against; its sub-patterns are matched against that remainder
(the value is also available via `URLParam(r, regexrouter.SubrouteParam)`):

```go
r.Route(`^/api/(?P<subroute>.*)$`, func(r regexrouter.Router) {
	r.Get(`^widgets$`, ...) // matches GET /api/widgets
})
```

If a `Route` pattern has no `subroute` group, its sub-router matches against the empty
string — useful when the sub-patterns are all `^$` (as in the OCI distribution routes).