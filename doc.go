// Package plugin is the Paca Backend Plugin SDK.
//
// Plugin authors implement the [Plugin] interface and call [Run] from their
// main function.  The SDK dispatcher wires up the WASM export functions
// (Init, HandleRequest, HandleEvent, Shutdown) and delegates to the plugin.
//
// # Minimal plugin skeleton
//
//	package main
//
//	import plugin "github.com/Paca-AI/plugin-sdk-go"
//
//	type myPlugin struct{}
//
//	func (p *myPlugin) Init(ctx *plugin.Context) error {
//	    ctx.Route("GET", "/items", p.listItems)
//	    ctx.On("task.deleted", p.onTaskDeleted)
//	    return nil
//	}
//
//	func (p *myPlugin) Shutdown() {}
//
//	func (p *myPlugin) listItems(req *plugin.Request, res *plugin.Response) {
//	    res.JSON(200, []string{"item1"})
//	}
//
//	func (p *myPlugin) onTaskDeleted(evt *plugin.Event) {
//	    // handle event
//	}
//
//	func main() { plugin.Run(&myPlugin{}) }
//
// # Architecture
//
// A backend plugin is compiled to a [WASM/WASI] module.  The paca host loads
// the module and communicates through a small set of host-provided import
// functions (declared in wasm_imports.go) and a matching set of exported
// functions (wasm_exports.go).  All host/guest communication uses JSON-encoded
// payloads exchanged over WASM linear memory.
//
// The [Context] type is the bridge between the plugin and the host.  It
// exposes typed helpers for SQL queries ([DB]), key-value storage ([KV]),
// structured logging ([Logger]), and configuration ([Config]).  During tests
// these are backed by in-memory implementations from the [plugintest] package.
package plugin
