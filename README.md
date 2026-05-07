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

Paca backend plugins must be compiled to WebAssembly using the WASI preview 1 target (`wasip1`).

```bash
GOARCH=wasm GOOS=wasip1 go build -o plugin.wasm main.go
```

> **Note**: Using [TinyGo](https://tinygo.org/) is strongly recommended for producing smaller WASM binaries (often < 1MB vs ~10MB with standard Go).

## API Reference

### `Context`
The bridge between your plugin and the Paca host. Available via `Init(ctx)`:
- `ctx.DB()` — access the scoped database (PostgreSQL)
- `ctx.KV()` — access the plugin-private key-value store (JSONB)
- `ctx.Log()` — write structured logs to the host
- `ctx.Config()` — read plugin configuration and secrets
- `ctx.Route(method, path, handler)` — register an HTTP endpoint
- `ctx.On(event, handler)` — subscribe to a core platform event

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
