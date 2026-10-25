// Package serve mounts a [gitprovider.Provider] on a plugin's routes.
package serve

import (
	"context"

	plugin "github.com/Paca-AI/plugin-sdk-go"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

// RoutePrefix is where provider operations are mounted. The host treats
// `/_git/*` as internal-only: browsers cannot reach it; only core services (and
// the MCP server, after core's own checks) call it.
const RoutePrefix = "/_git"

// Register mounts every operation as POST /_git/:op. Call it from the plugin's
// Init:
//
//	func (p *myPlugin) Init(ctx *plugin.Context) error {
//	    serve.Register(ctx, newMyProvider(ctx))
//	    return nil
//	}
func Register(ctx *plugin.Context, p gitprovider.Provider) {
	ctx.Route("POST", RoutePrefix+"/:op", func(req *plugin.Request, res *plugin.Response) {
		status, body := gitprovider.Dispatch(context.Background(), p, req.PathParam("op"), req.Body)
		res.StatusCode = status
		res.Headers["Content-Type"] = "application/json"
		res.Body = body
	})
}
