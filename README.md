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
	r := regexrouter.New(nil)
	r.Get(`^\/$`, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	r.Get(`^\/(?P<var1>.*)\/(?P<var2>.*)\/path$`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%s %s", r.Context().Value("var1"), r.Context().Value("var2"))))
	})
	http.ListenAndServe(":3000", r)
}
```