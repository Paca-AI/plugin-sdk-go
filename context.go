package plugin

import "strings"

// Context is passed to [Plugin.Init] and used to register route handlers and
// event subscriptions.  It also gives access to platform services such as the
// database, key-value store, logger, and configuration.
type Context struct {
	routes map[routeKey]RouteHandler
	events map[string]EventHandler
	db     *DB
	kv     *KV
	log    *Logger
	cfg    *Config
}

// routeKey uniquely identifies a registered route by HTTP method + path.
type routeKey struct {
	method string
	path   string
}

// Route registers a handler for the given HTTP method and path.
// Paths are relative to the plugin's base URL:
//
//	/api/v1/plugins/{pluginId}/projects/:projectId/{path}
//
// Path parameters are available via [Request.PathParam].
func (c *Context) Route(method, path string, handler RouteHandler) {
	c.routes[routeKey{strings.ToUpper(method), path}] = handler
}

// On registers an event handler for the given topic.
//
// Example topics: "task.created", "task.deleted", "member.added".
func (c *Context) On(topic string, handler EventHandler) {
	c.events[topic] = handler
}

// DB returns a helper for typed SQL operations scoped to the plugin schema.
func (c *Context) DB() *DB { return c.db }

// KV returns a helper for simple key-value persistence.
func (c *Context) KV() *KV { return c.kv }

// Log returns a structured logger.
func (c *Context) Log() *Logger { return c.log }

// Config returns a read-only helper for plugin configuration values.
func (c *Context) Config() *Config { return c.cfg }

// RouteHandler is the function signature for HTTP route handlers.
type RouteHandler func(req *Request, res *Response)

// EventHandler is the function signature for event subscription handlers.
type EventHandler func(evt *Event)

// newContext constructs a Context backed by the provided implementations.
// Called by the WASM runtime (with host-function backends) and by
// [plugintest] (with in-memory backends).
func newContext(db DBBackend, kv KVBackend, log LogBackend, cfg ConfigBackend) *Context {
	return &Context{
		routes: make(map[routeKey]RouteHandler),
		events: make(map[string]EventHandler),
		db:     &DB{backend: db},
		kv:     &KV{backend: kv},
		log:    &Logger{backend: log},
		cfg:    &Config{backend: cfg},
	}
}

func (c *Context) matchRoute(method, path string) (RouteHandler, map[string]string, bool) {
	method = strings.ToUpper(method)
	if handler, ok := c.routes[routeKey{method, path}]; ok {
		return handler, make(map[string]string), true
	}

	projectID, relativePath, hasProjectScope := splitProjectPath(path)

	for key, handler := range c.routes {
		if key.method != method {
			continue
		}
		if params, ok := matchRoutePattern(key.path, path); ok {
			return handler, params, true
		}
		if hasProjectScope {
			if params, ok := matchRoutePattern(key.path, relativePath); ok {
				params["projectId"] = projectID
				return handler, params, true
			}
		}
	}

	return nil, nil, false
}

func splitProjectPath(path string) (projectID, relativePath string, ok bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", false
	}

	segments := strings.Split(trimmed, "/")
	if len(segments) < 2 || segments[0] != "projects" {
		return "", "", false
	}

	projectID = segments[1]
	if len(segments) == 2 {
		return projectID, "/", true
	}
	return projectID, "/" + strings.Join(segments[2:], "/"), true
}

func matchRoutePattern(pattern, path string) (map[string]string, bool) {
	patternSegments := splitPathSegments(pattern)
	pathSegments := splitPathSegments(path)
	if len(patternSegments) != len(pathSegments) {
		return nil, false
	}

	params := make(map[string]string)
	for i := range patternSegments {
		patternSegment := patternSegments[i]
		pathSegment := pathSegments[i]

		if strings.HasPrefix(patternSegment, ":") {
			name := strings.TrimPrefix(patternSegment, ":")
			if name == "" {
				return nil, false
			}
			params[name] = pathSegment
			continue
		}

		if patternSegment != pathSegment {
			return nil, false
		}
	}

	return params, true
}

func splitPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
