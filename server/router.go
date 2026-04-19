package server

import (
	"strings"
	"sync"
)

// Router is a trie-based HTTP URL router with support for dynamic
// parameters (:param), wildcards (*path), and middleware chains.
type Router struct {
	trees      map[string]*trieNode // method → trie root
	middleware []HandlerFunc        // global middleware
	mu         sync.RWMutex

	// NotFoundHandler is called when no route matches.
	NotFoundHandler HandlerFunc
	// MethodNotAllowedHandler is called when a route exists but not for the given method.
	MethodNotAllowedHandler HandlerFunc
}

// trieNode is a single node in the routing trie.
type trieNode struct {
	// segment is the static path segment this node matches.
	segment string
	// children are static child nodes.
	children map[string]*trieNode
	// paramChild is the child node for a dynamic parameter (:name).
	paramChild *trieNode
	// paramName is the name of the dynamic parameter (without :).
	paramName string
	// wildcardChild is the child node for a wildcard segment (*name).
	wildcardChild *trieNode
	// wildcardName is the name of the wildcard parameter (without *).
	wildcardName string
	// handlers is the handler chain (middleware + handler) for this endpoint.
	handlers []HandlerFunc
	// hasHandler indicates whether this node has a registered handler.
	hasHandler bool
}

// RouteGroup is a group of routes sharing a common prefix and middleware.
type RouteGroup struct {
	prefix     string
	router     *Router
	middleware []HandlerFunc
}

// NewRouter creates a new Router with default handlers.
func NewRouter() *Router {
	r := &Router{
		trees: make(map[string]*trieNode),
	}

	r.NotFoundHandler = func(ctx *Context) {
		ctx.JSON(404, map[string]interface{}{
			"error":   "Not Found",
			"message": "The requested resource was not found",
			"status":  404,
		})
	}

	r.MethodNotAllowedHandler = func(ctx *Context) {
		ctx.JSON(405, map[string]interface{}{
			"error":   "Method Not Allowed",
			"message": "The request method is not supported for this resource",
			"status":  405,
		})
	}

	return r
}

// Use registers global middleware that runs on every request.
func (r *Router) Use(middleware ...HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middleware = append(r.middleware, middleware...)
}

// Group creates a new route group with the given prefix.
func (r *Router) Group(prefix string, middleware ...HandlerFunc) *RouteGroup {
	return &RouteGroup{
		prefix:     prefix,
		router:     r,
		middleware: middleware,
	}
}

// GET registers a route for HTTP GET.
func (r *Router) GET(path string, handlers ...HandlerFunc) {
	r.addRoute("GET", path, handlers)
}

// POST registers a route for HTTP POST.
func (r *Router) POST(path string, handlers ...HandlerFunc) {
	r.addRoute("POST", path, handlers)
}

// PUT registers a route for HTTP PUT.
func (r *Router) PUT(path string, handlers ...HandlerFunc) {
	r.addRoute("PUT", path, handlers)
}

// PATCH registers a route for HTTP PATCH.
func (r *Router) PATCH(path string, handlers ...HandlerFunc) {
	r.addRoute("PATCH", path, handlers)
}

// DELETE registers a route for HTTP DELETE.
func (r *Router) DELETE(path string, handlers ...HandlerFunc) {
	r.addRoute("DELETE", path, handlers)
}

// OPTIONS registers a route for HTTP OPTIONS.
func (r *Router) OPTIONS(path string, handlers ...HandlerFunc) {
	r.addRoute("OPTIONS", path, handlers)
}

// HEAD registers a route for HTTP HEAD.
func (r *Router) HEAD(path string, handlers ...HandlerFunc) {
	r.addRoute("HEAD", path, handlers)
}

// Handle registers a route for any HTTP method.
func (r *Router) Handle(method, path string, handlers ...HandlerFunc) {
	r.addRoute(method, path, handlers)
}

// ---- Route Group Methods ----

// Use registers middleware for this group.
func (g *RouteGroup) Use(middleware ...HandlerFunc) {
	g.middleware = append(g.middleware, middleware...)
}

// Group creates a nested sub-group.
func (g *RouteGroup) Group(prefix string, middleware ...HandlerFunc) *RouteGroup {
	return &RouteGroup{
		prefix:     g.prefix + prefix,
		router:     g.router,
		middleware: append(g.middleware, middleware...),
	}
}

// GET registers a GET route within this group.
func (g *RouteGroup) GET(path string, handlers ...HandlerFunc) {
	g.handle("GET", path, handlers)
}

// POST registers a POST route within this group.
func (g *RouteGroup) POST(path string, handlers ...HandlerFunc) {
	g.handle("POST", path, handlers)
}

// PUT registers a PUT route within this group.
func (g *RouteGroup) PUT(path string, handlers ...HandlerFunc) {
	g.handle("PUT", path, handlers)
}

// PATCH registers a PATCH route within this group.
func (g *RouteGroup) PATCH(path string, handlers ...HandlerFunc) {
	g.handle("PATCH", path, handlers)
}

// DELETE registers a DELETE route within this group.
func (g *RouteGroup) DELETE(path string, handlers ...HandlerFunc) {
	g.handle("DELETE", path, handlers)
}

// handle is the internal method that prepends group prefix and middleware.
func (g *RouteGroup) handle(method, path string, handlers []HandlerFunc) {
	fullPath := g.prefix + path
	allHandlers := make([]HandlerFunc, 0, len(g.middleware)+len(handlers))
	allHandlers = append(allHandlers, g.middleware...)
	allHandlers = append(allHandlers, handlers...)
	g.router.addRoute(method, fullPath, allHandlers)
}

// ---- Trie Operations ----

// addRoute inserts a new route into the trie.
func (r *Router) addRoute(method, path string, handlers []HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if path == "" || path[0] != '/' {
		path = "/" + path
	}

	root, ok := r.trees[method]
	if !ok {
		root = &trieNode{
			children: make(map[string]*trieNode),
		}
		r.trees[method] = root
	}

	// Handle root path
	if path == "/" {
		root.handlers = handlers
		root.hasHandler = true
		return
	}

	segments := splitPath(path)
	current := root

	for _, segment := range segments {
		if segment == "" {
			continue
		}

		if segment[0] == ':' {
			// Dynamic parameter
			paramName := segment[1:]
			if current.paramChild == nil {
				current.paramChild = &trieNode{
					segment:  segment,
					children: make(map[string]*trieNode),
				}
			}
			current.paramChild.paramName = paramName
			current = current.paramChild
		} else if segment[0] == '*' {
			// Wildcard
			wildcardName := segment[1:]
			if wildcardName == "" {
				wildcardName = "wildcard"
			}
			current.wildcardChild = &trieNode{
				segment:      segment,
				wildcardName: wildcardName,
				children:     make(map[string]*trieNode),
			}
			current = current.wildcardChild
			// Wildcard must be the last segment
			break
		} else {
			// Static segment
			child, ok := current.children[segment]
			if !ok {
				child = &trieNode{
					segment:  segment,
					children: make(map[string]*trieNode),
				}
				current.children[segment] = child
			}
			current = child
		}
	}

	current.handlers = handlers
	current.hasHandler = true
}

// lookup finds the matching handlers and extracts path parameters.
func (r *Router) lookup(method, path string) ([]HandlerFunc, map[string]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	root, ok := r.trees[method]
	if !ok {
		return nil, nil, false
	}

	// Handle root path
	if path == "/" {
		if root.hasHandler {
			return root.handlers, nil, true
		}
		return nil, nil, false
	}

	segments := splitPath(path)
	params := make(map[string]string)

	handlers, found := r.search(root, segments, 0, params)
	if found {
		return handlers, params, true
	}

	return nil, nil, false
}

// search recursively traverses the trie to find a matching route.
func (r *Router) search(node *trieNode, segments []string, depth int, params map[string]string) ([]HandlerFunc, bool) {
	// Base case: consumed all segments
	if depth == len(segments) {
		if node.hasHandler {
			return node.handlers, true
		}
		return nil, false
	}

	segment := segments[depth]

	// Try static match first (highest priority)
	if child, ok := node.children[segment]; ok {
		if handlers, found := r.search(child, segments, depth+1, params); found {
			return handlers, true
		}
	}

	// Try dynamic parameter match
	if node.paramChild != nil {
		params[node.paramChild.paramName] = segment
		if handlers, found := r.search(node.paramChild, segments, depth+1, params); found {
			return handlers, true
		}
		delete(params, node.paramChild.paramName) // Backtrack
	}

	// Try wildcard match (captures rest of path)
	if node.wildcardChild != nil {
		remaining := strings.Join(segments[depth:], "/")
		params[node.wildcardChild.wildcardName] = remaining
		if node.wildcardChild.hasHandler {
			return node.wildcardChild.handlers, true
		}
	}

	return nil, false
}

// hasAnyRoute checks if any route exists for the given path (any method).
func (r *Router) hasAnyRoute(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, root := range r.trees {
		params := make(map[string]string)
		if _, found := r.search(root, splitPath(path), 0, params); found {
			return true
		}
	}
	return false
}

// ServeHTTP is the main entry point for routing a request.
// It looks up the route, builds the middleware chain, and executes it.
func (r *Router) ServeHTTP(ctx *Context) {
	path := ctx.Request.Path
	method := ctx.Request.Method

	// Look up the route
	handlers, params, found := r.lookup(method, path)
	if !found {
		// Check if any method matches (for 405)
		if r.hasAnyRoute(path) {
			// Build chain with global middleware + 405 handler
			chain := r.buildChain([]HandlerFunc{r.MethodNotAllowedHandler})
			ctx.setHandlers(chain)
			ctx.Next()
			return
		}

		// No route at all — 404
		chain := r.buildChain([]HandlerFunc{r.NotFoundHandler})
		ctx.setHandlers(chain)
		ctx.Next()
		return
	}

	// Set path parameters
	if params != nil {
		ctx.Params = params
	}

	// Build full chain: global middleware → route middleware + handler
	chain := r.buildChain(handlers)
	ctx.setHandlers(chain)
	ctx.Next()
}

// buildChain prepends global middleware to the handler chain.
func (r *Router) buildChain(handlers []HandlerFunc) []HandlerFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain := make([]HandlerFunc, 0, len(r.middleware)+len(handlers))
	chain = append(chain, r.middleware...)
	chain = append(chain, handlers...)
	return chain
}

// splitPath splits a URL path into segments, removing empty segments.
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			segments = append(segments, p)
		}
	}
	return segments
}
