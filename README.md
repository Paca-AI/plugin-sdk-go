# github.com/Paca-AI/plugin-sdk-go — Backend

Go SDK for building Paca backend plugins compiled to WebAssembly (WASM).

## Installation

```bash
go get github.com/Paca-AI/plugin-sdk-go
```

## Quick Start

Every backend plugin must implement the `Plugin` interface and call `plugin.Run` in its `main` function.

```go
//go:build wasip1

package main

import (
    "fmt"
    plugin "github.com/Paca-AI/plugin-sdk-go"
)

type myPlugin struct {
    db  *plugin.DB
    log *plugin.Logger
}

func (p *myPlugin) Init(ctx *plugin.Context) error {
    p.db = ctx.DB()
    p.log = ctx.Log()

    // Register HTTP routes
    ctx.Route("GET", "/hello", p.sayHello)

    // Subscribe to platform events
    ctx.On("task.created", p.onTaskCreated)

    return nil
}

func (p *myPlugin) Shutdown() {}

func (p *myPlugin) sayHello(req *plugin.Request, res *plugin.Response) {
    name := req.QueryParam("name")
    if name == "" {
        name = "World"
    }
    res.JSON(200, map[string]string{"message": fmt.Sprintf("Hello, %s!", name)})
}

func (p *myPlugin) onTaskCreated(evt *plugin.Event) {
    p.log.Info("A new task was created!")
}

func main() {
    plugin.Run(&myPlugin{})
}
```

## Building for Paca

Paca backend plugins must be compiled to WebAssembly using the WASI preview 1 target (`wasip1`), with TinyGo:

```bash
tinygo build -target=wasip1 -buildmode=c-shared -o plugin.wasm main.go
```

`-buildmode=c-shared` is required, not optional: without it, TinyGo doesn't
wire up the WASI reactor entry point (`_initialize`) that this SDK's
runtime depends on, and every call into the plugin panics at runtime with
`"//go:wasmexport function called before runtime initialization"` — it
still compiles fine without the flag, so this only surfaces once the
plugin is actually loaded by the host.

TinyGo is strongly recommended over the standard Go compiler: it doesn't
statically link Go's full runtime (GC, scheduler, reflection), so a typical
plugin binary is well under 1MB versus several MB with standard Go — and
since every loaded plugin gets its own independent copy of that runtime on
the host, this is a real difference in the host's memory use per plugin,
not just download size.

Standard Go 1.24+ also works (`GOARCH=wasm GOOS=wasip1 go build
-buildmode=c-shared -o plugin.wasm main.go`) and is the right fallback if
your plugin hits a TinyGo compatibility limitation — TinyGo implements a
subset of the stdlib, notably around `reflect`. One TinyGo-specific
constraint either way: if your plugin declares its own `//go:wasmimport`
host function beyond what this SDK provides (see [`CallHostFunction`](./custom_host_call.go)),
call it directly by name rather than passing it as a value — TinyGo
doesn't support taking the address of a `go:wasmimport` function, only
calling it. If you use `CallHostFunction`, wrap your import in a small
closure:

```go
// Works under both standard Go and TinyGo:
err := plugin.CallHostFunction(func(a, b, c, d int64) { hostMyImport(a, b, c, d) }, req, &res)

// Compiles under standard Go, but fails to build under TinyGo:
err := plugin.CallHostFunction(hostMyImport, req, &res)
```

## API Reference

### `Context`
The bridge between your plugin and the Paca host. Available via `Init(ctx)`:
- `ctx.DB()` — access the scoped database (PostgreSQL)
- `ctx.KV()` — access the plugin-private key-value store (JSONB), durable with no expiry
- `ctx.Cache()` — access the plugin's TTL-based cache (Valkey/Redis), for derived data that can tolerate being briefly stale
- `ctx.Log()` — write structured logs to the host
- `ctx.Config()` — read plugin configuration and secrets
- `ctx.Route(method, path, handler)` — register an HTTP endpoint
- `ctx.On(event, handler)` — subscribe to a core platform event

Route-level host middleware (for example `authn`, `requireFreshPassword`,
`requirePermissions`) is configured in `plugin.json` under
`backend.routes[].middlewares`.

### `DB`
Typed SQL execution. The host enforces row-level scoping; you can only see data for the project where the plugin is enabled.
- `db.Query(sql, args...)` — execute a read query; returns `[]map[string]any`
- `db.Exec(sql, args...)` — execute an insert/update/delete
- `db.Begin()` — start a database transaction

### `Request` & `Response`
Idiomatic HTTP handling inside your route handlers:
- `req.PathParam(name)` — get a URL path parameter (e.g., `:taskId`)
- `req.QueryParam(name)` — get a URL query parameter
- `plugin.JSONBody[T](req)` — parse the JSON request body into a struct
- `res.JSON(code, data)` — write a JSON response
- `res.Error(code, message)` — write a standard error response: `{"error": "..."}`
- `res.NoContent()` — return a 204 No Content response

## Unit Testing

The SDK includes the `plugintest` package for testing your plugin logic without a running Paca instance.

```go
import (
    "testing"
    "github.com/Paca-AI/plugin-sdk-go/plugintest"
)

func TestHello(t *testing.T) {
    tc := plugintest.NewContext(t)
    
    var p myPlugin
    p.Init(tc.PluginContext())

    res := tc.Call("GET", "/hello", plugintest.Request{
        Query: map[string]string{"name": "Paca"},
    })

    if res.StatusCode != 200 {
        t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
    }
}
```

## License

MIT
