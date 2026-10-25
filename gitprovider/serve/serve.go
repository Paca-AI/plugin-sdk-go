// Package serve mounts a [gitprovider.Provider] on a plugin's routes.
package serve

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	plugin "github.com/Paca-AI/plugin-sdk-go"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

// RoutePrefix is where provider operations are mounted. The host treats
// `/_git/*` as internal-only: browsers cannot reach it; only core services (and
// the MCP server, after core's own checks) call it.
const RoutePrefix = "/_git"

// callTimeout bounds one operation: plugin.Request carries no context, so
// without it a stalled platform call would outlive the caller's own timeout.
const callTimeout = 60 * time.Second

// Register mounts every operation as POST /_git/:op. Call it from the plugin's
// Init:
//
//	func (p *myPlugin) Init(ctx *plugin.Context) error {
//	    serve.Register(ctx, newMyProvider(ctx))
//	    return nil
//	}
//
// The route is only safe because the host refuses to expose `/_git/*` to
// browsers; the SDK performs no caller authentication of its own.
func Register(ctx *plugin.Context, p gitprovider.Provider) {
	ctx.Route("POST", RoutePrefix+"/:op", func(req *plugin.Request, res *plugin.Response) {
		cctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		status, body := gitprovider.Dispatch(cctx, p, req.PathParam("op"), req.Body)
		res.StatusCode = status
		res.Headers["Content-Type"] = "application/json"
		if status == 429 || status == 503 {
			var env gitprovider.Envelope
			if json.Unmarshal(body, &env) == nil && env.Error != nil && env.Error.RetryAfterSeconds > 0 {
				res.Headers["Retry-After"] = strconv.Itoa(env.Error.RetryAfterSeconds)
			}
		}
		res.Body = body
	})
}
