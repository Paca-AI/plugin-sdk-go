package plugin

import "encoding/json"

// CallerIdentity contains the authenticated identity of the HTTP caller,
// forwarded by the host on every request.
type CallerIdentity struct {
	// CallerID is the project_member UUID of the caller.
	CallerID string `json:"caller_id"`
	// CallerRole is the role name of the caller within the project.
	CallerRole string `json:"caller_role"`
	// ProjectID is the project the request is scoped to.
	ProjectID string `json:"project_id"`
}

// Request represents the inbound HTTP request forwarded from the host.
type Request struct {
	// Method is the HTTP verb in upper-case (e.g. "GET", "POST").
	Method string
	// Path is the matched route path relative to the plugin base URL.
	Path string
	// PathParams contains named route parameter values, keyed without the
	// leading colon (e.g. route "/:id" → PathParams["id"]).
	PathParams map[string]string
	// Query contains URL query parameter values.
	Query map[string]string
	// Headers is a flat map of request headers (lower-cased keys).
	Headers map[string]string
	// Body is the raw request body bytes.
	Body []byte
	// Caller is the authenticated identity of the caller.
	Caller CallerIdentity
}

// PathParam returns the value of a named path parameter (e.g. "id").
// Returns an empty string if the parameter does not exist.
func (r *Request) PathParam(name string) string { return r.PathParams[name] }

// QueryParam returns the value of a URL query parameter.
// Returns an empty string if the parameter does not exist.
func (r *Request) QueryParam(name string) string { return r.Query[name] }

// JSONBody decodes the request body as JSON into a value of type T.
// Returns the zero value of T when the body is empty.
func JSONBody[T any](req *Request) (T, error) {
	var dst T
	if len(req.Body) == 0 {
		return dst, nil
	}
	err := json.Unmarshal(req.Body, &dst)
	return dst, err
}
