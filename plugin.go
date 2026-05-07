package plugin

// Plugin is the interface every backend plugin must implement.
type Plugin interface {
	// Init is called once when the plugin is loaded.  Register route handlers
	// and event subscriptions on the provided Context.
	Init(ctx *Context) error

	// Shutdown is called before the plugin module is unloaded.  Use it to
	// flush buffers or close any open resources.
	Shutdown()
}

// globalDispatcher is the singleton created by [Run].
//nolint:unused // used by wasm_exports.go in WASM builds
var globalDispatcher *dispatcher

// Run wires the plugin into the WASM host function contract.  Call it from
// main() with the concrete Plugin implementation:
//
//	func main() { plugin.Run(&myPlugin{}) }
func Run(p Plugin) {
	globalDispatcher = newDispatcher(p)
}
