package plugin

import "encoding/json"

// Response is the HTTP response the plugin builds; the host serialises it
// back to the HTTP client.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// NewResponse creates a Response with status 200 and empty headers.
func NewResponse() *Response {
	return &Response{
		StatusCode: 200,
		Headers:    make(map[string]string),
	}
}

// JSON sets the response status, marshals body as JSON, and sets the
// Content-Type header to "application/json".
func (r *Response) JSON(status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		r.StatusCode = 500
		r.Body = []byte(`{"error":"internal serialisation error"}`)
		return
	}
	r.StatusCode = status
	r.Headers["Content-Type"] = "application/json"
	r.Body = data
}

// Text sets the response status and a plain-text body.
func (r *Response) Text(status int, text string) {
	r.StatusCode = status
	r.Headers["Content-Type"] = "text/plain"
	r.Body = []byte(text)
}

// NoContent sets the status to 204 with no body.
func (r *Response) NoContent() {
	r.StatusCode = 204
	r.Body = nil
}

// Error writes a JSON error response: {"error": message}.
func (r *Response) Error(status int, message string) {
	r.JSON(status, map[string]string{"error": message})
}

// BodyString returns the response body as a string.
// Useful for logging and assertions in tests.
func (r *Response) BodyString() string { return string(r.Body) }
