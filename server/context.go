package server

import (
	"encoding/json"
	"net"
	"sync"
)

// HandlerFunc is the type for request handlers in Forge.
type HandlerFunc func(ctx *Context)

// Context is the per-request context object that provides access to the request,
// response, path parameters, and middleware-shared values.
type Context struct {
	// Request is the parsed HTTP request.
	Request *Request
	// Response is the response builder.
	Response *Response
	// Params holds URL path parameters (e.g., :id → "123").
	Params map[string]string
	// conn is the underlying TCP connection.
	conn net.Conn

	// store holds arbitrary key-value pairs set by middleware.
	store map[string]interface{}
	mu    sync.RWMutex

	// index tracks the current position in the middleware chain.
	index    int
	handlers []HandlerFunc

	// aborted flags that the middleware chain should stop.
	aborted bool

	// hijacked indicates the connection has been taken over (e.g., for WebSocket).
	// When true, the server will NOT write a response or close the connection.
	hijacked bool
}

// NewContext creates a new Context for a request.
func NewContext(req *Request, resp *Response, conn net.Conn) *Context {
	return &Context{
		Request:  req,
		Response: resp,
		Params:   make(map[string]string),
		conn:     conn,
		store:    make(map[string]interface{}),
		index:    -1,
	}
}

// ---- Request Helpers ----

// Method returns the HTTP method.
func (c *Context) Method() string {
	return c.Request.Method
}

// Path returns the URL path.
func (c *Context) Path() string {
	return c.Request.Path
}

// QueryParam returns a query string parameter by name.
func (c *Context) QueryParam(name string) string {
	return c.Request.Query.Get(name)
}

// QueryParamDefault returns a query parameter with a default fallback.
func (c *Context) QueryParamDefault(name, defaultValue string) string {
	v := c.Request.Query.Get(name)
	if v == "" {
		return defaultValue
	}
	return v
}

// Param returns a URL path parameter by name.
func (c *Context) Param(name string) string {
	return c.Params[name]
}

// Header returns a request header value (case-insensitive).
func (c *Context) Header(name string) string {
	return c.Request.GetHeader(name)
}

// RemoteAddr returns the client's address.
func (c *Context) RemoteAddr() string {
	return c.Request.RemoteAddr
}

// BodyBytes returns the raw request body.
func (c *Context) BodyBytes() []byte {
	return c.Request.Body
}

// BindJSON parses the request body as JSON into the target value.
func (c *Context) BindJSON(v interface{}) error {
	return json.Unmarshal(c.Request.Body, v)
}

// ---- Response Helpers ----

// Status sets the response status code and returns the context for chaining.
func (c *Context) Status(code int) *Context {
	c.Response.SetStatus(code)
	return c
}

// JSON sends a JSON response with the given status code.
func (c *Context) JSON(code int, data interface{}) {
	c.Response.SetStatus(code)
	c.Response.SetHeader("Content-Type", "application/json; charset=utf-8")

	body, err := json.Marshal(data)
	if err != nil {
		c.Response.SetStatus(500)
		body = []byte(`{"error":"internal server error"}`)
	}
	c.Response.SetBody(body)
}

// Text sends a plain text response.
func (c *Context) Text(code int, text string) {
	c.Response.SetStatus(code)
	c.Response.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.Response.SetBody([]byte(text))
}

// HTML sends an HTML response.
func (c *Context) HTML(code int, html string) {
	c.Response.SetStatus(code)
	c.Response.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.Response.SetBody([]byte(html))
}

// Error sends a standardized error response.
func (c *Context) Error(code int, message string) {
	c.JSON(code, map[string]interface{}{
		"error":   StatusText(code),
		"message": message,
		"status":  code,
	})
}

// NoContent sends a 204 No Content response.
func (c *Context) NoContent() {
	c.Response.SetStatus(204)
}

// Redirect sends a redirect response.
func (c *Context) Redirect(code int, url string) {
	c.Response.SetStatus(code)
	c.Response.SetHeader("Location", url)
}

// SetResponseHeader sets a response header.
func (c *Context) SetResponseHeader(name, value string) {
	c.Response.SetHeader(name, value)
}

// ---- Middleware Store ----

// Set stores a key-value pair in the context (thread-safe).
// Used by middleware to pass values to handlers.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
}

// Get retrieves a value from the context store (thread-safe).
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.store[key]
	return val, ok
}

// GetString retrieves a string value from the context store.
func (c *Context) GetString(key string) string {
	val, ok := c.Get(key)
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// ---- Middleware Chain ----

// Next executes the next handler in the middleware chain.
func (c *Context) Next() {
	c.index++
	for c.index < len(c.handlers) {
		if c.aborted {
			return
		}
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort stops the middleware chain from continuing.
// The current handler will still complete, but no subsequent handlers will run.
func (c *Context) Abort() {
	c.aborted = true
}

// AbortWithError stops the chain and sends an error response.
func (c *Context) AbortWithError(code int, message string) {
	c.Error(code, message)
	c.Abort()
}

// IsAborted returns whether the chain has been aborted.
func (c *Context) IsAborted() bool {
	return c.aborted
}

// setHandlers sets the middleware + handler chain for this context.
func (c *Context) setHandlers(handlers []HandlerFunc) {
	c.handlers = handlers
}

// ---- Connection Hijack (for WebSocket) ----

// Hijack takes over the underlying TCP connection.
// After calling Hijack, the server will NOT write an HTTP response or close the connection.
// The caller becomes responsible for the connection lifecycle.
func (c *Context) Hijack() net.Conn {
	c.hijacked = true
	return c.conn
}

// Conn returns the raw TCP connection without hijacking.
func (c *Context) Conn() net.Conn {
	return c.conn
}

// IsHijacked returns whether the connection has been hijacked.
func (c *Context) IsHijacked() bool {
	return c.hijacked
}
