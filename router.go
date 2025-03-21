package ruta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	Default    = "default"
	Slash      = "/"
	Empty      = ""
	Open       = "{"
	Close      = "}"
	Param      = "param"
	ErrorLimit = 10
)

// Connection defines the interface for network connections, such as WebSocket or TCP.
// Implementations must support reading, writing, and closing the connection.
type Connection interface {
	Read() ([]byte, error)
	Write(message []byte) error
	Close() error
}

// Handler defines the signature for route handlers.
// It processes a Frame containing request data and connection details.
type Handler func(*Frame)

// Frame encapsulates data for a single routing request.
// It is thread-safe for concurrent access to Params and Errors.
type Frame struct {
	Conn    Connection             // Connection to the client
	Params  *Params                // Route parameters (e.g., {"id": "123"})
	Values  map[string]interface{} // User-defined values for middleware or handlers
	Errors  []error                // Accumulated errors during processing
	Route   string                 // Matched route pattern (e.g., "/user/{id}")
	mu      sync.Mutex             // Protects Errors and Values from concurrent access
	Payload *Payload               // Payload
}

// Command extracts the command from the payload.
func (f *Frame) Command() string {
	return f.Payload.Command()
}

// Payload represents the message payload with command and data
type Payload struct {
	command string    // Extracted command
	payload io.Reader // Payload data as a stream
}

// NewPayload creates a new Payload instance from the raw message
func NewPayload(rawMessage []byte) *Payload {
	parts := strings.SplitN(string(rawMessage), " ", 2)
	command := parts[0]
	var payload io.Reader
	if len(parts) > 1 {
		payload = bytes.NewReader([]byte(parts[1]))
	} else {
		payload = bytes.NewReader([]byte(""))
	}
	return &Payload{command: command, payload: payload}
}

// Command returns the extracted command
func (p *Payload) Command() string {
	return p.command
}

// Payload returns the payload as an io.Reader
func (p *Payload) Payload() io.Reader {
	return p.payload
}

// Text reads the payload as a string
func (p *Payload) Text() (string, error) {
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, p.payload)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// JSON decodes the payload as JSON into a target struct
func (p *Payload) JSON(v interface{}) error {
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, p.payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), v)
}

// Bytes reads the payload as raw bytes
func (p *Payload) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, p.payload)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RouteBase defines the contract for routing functionality.
// Implementations must support middleware, route registration, and request handling.
type RouteBase interface {
	// Use appends middleware to all routes in this router.
	Use(middleware ...func(*Frame) error)

	// Route registers a handler for a specific pattern (e.g., "/user/{id}").
	Route(pattern string, handler Handler)

	// Section groups routes under a common prefix (e.g., "/api/").
	Section(prefix string, fn func(RouteBase)) RouteBase

	// Group scopes middleware to a subset of routes.
	Group(fn func(RouteBase)) RouteBase

	// With creates a new router with additional middleware.
	With(middleware ...func(*Frame) error) RouteBase

	// Handle processes an incoming message and invokes the matching handler.
	Handle(ctx *Frame) error

	// Routes returns all registered route patterns.
	Routes() []string
}

// Params provides thread-safe storage for route parameters.
type Params struct {
	data sync.Map
}

// NewParams creates a new, empty Params instance.
func NewParams() *Params {
	return &Params{}
}

// Set stores a key-value pair in the parameters.
// It is safe for concurrent use.
func (p *Params) Set(key, value string) {
	p.data.Store(key, value)
}

// Get retrieves a parameter by key.
// It returns the value and a boolean indicating if the key exists.
func (p *Params) Get(key string) (string, bool) {
	val, ok := p.data.Load(key)
	if !ok {
		return Empty, false
	}
	return val.(string), true
}

// Node represents a single node in the routing trie.
// It supports static paths and parameterized routes (e.g., "{id}").
type Node struct {
	children  map[string]*Node // Child nodes by path segment
	handler   Handler          // Handler for this route, if any
	isParam   bool             // True if this node represents a parameter
	paramName string           // Name of the parameter (e.g., "id")
}

// NewNode initializes an empty trie node.
func NewNode() *Node {
	return &Node{children: make(map[string]*Node)}
}

// AddChild adds a child node for a given path segment.
// If isParam is true, paramName is stored (e.g., "id" for "{id}").
// Returns the child node, reusing an existing one if present.
func (n *Node) AddChild(key string, isParam bool, paramName string) *Node {
	if _, exists := n.children[key]; !exists {
		n.children[key] = &Node{
			children:  make(map[string]*Node),
			isParam:   isParam,
			paramName: paramName,
		}
	}
	return n.children[key]
}

// FindHandler traverses the trie to find a handler for the given path parts.
// It extracts parameters (e.g., "123" for "{id}") and returns them with the handler.
// Returns nil if no handler matches.
func (n *Node) FindHandler(parts []string) (Handler, *Params) {
	params := NewParams()
	node := n

	for i, part := range parts {
		if part == Empty && i == len(parts)-1 { // Skip trailing empty part
			break
		}

		if nextNode, exists := node.children[part]; exists {
			node = nextNode
		} else if paramNode, exists := node.children["{param}"]; exists && paramNode.isParam {
			params.Set(paramNode.paramName, part)
			node = paramNode
		} else {
			return nil, nil
		}
	}

	if node.handler != nil {
		return node.handler, params
	}
	return nil, nil
}

// Router is the concrete implementation of RouteBase.
// It uses a trie for efficient route matching and supports middleware and nesting.
type Router struct {
	root       *Node                // Root of the route trie
	middleware []func(*Frame) error // Middleware stack
	pool       sync.Pool            // Pool for Frame reuse
	prefix     string               // Base prefix for this router (e.g., "/api/")
}

// NewRouter constructs a new Router instance.
// It initializes a trie and a Frame pool for efficient memory use.
func NewRouter() RouteBase {
	r := &Router{
		root: NewNode(),
	}
	r.pool = sync.Pool{
		New: func() interface{} {
			return &Frame{
				Params: NewParams(),
				Values: make(map[string]interface{}),
			}
		},
	}
	return r
}

// Use appends middleware to the router’s stack, applied to all routes.
func (r *Router) Use(middleware ...func(*Frame) error) {
	r.middleware = append(r.middleware, middleware...)
}

// With returns a new Router with additional middleware.
// It preserves the trie and prefix from the parent router.
func (r *Router) With(middleware ...func(*Frame) error) RouteBase {
	return &Router{
		root:       r.root,
		middleware: append(append([]func(*Frame) error{}, r.middleware...), middleware...),
		pool:       r.pool,
		prefix:     r.prefix,
	}
}

// Group creates a scoped router with additional middleware.
// It applies the middleware only to routes defined within the group.
func (r *Router) Group(fn func(RouteBase)) RouteBase {
	newRouter := r.With()
	fn(newRouter)
	return newRouter
}

// Section groups routes under a common prefix (e.g., "/api/").
// Nested sections accumulate prefixes (e.g., "/api/v1/").
func (r *Router) Section(prefix string, fn func(RouteBase)) RouteBase {
	prefix = strings.Trim(prefix, Slash) + Slash
	fullPrefix := r.prefix + prefix
	fn(&Router{
		root:       r.root,
		middleware: r.middleware,
		pool:       r.pool,
		prefix:     fullPrefix,
	})
	return r
}

// Route registers a handler for a given pattern.
// Use "default", / , or Empty for the base route at the current prefix.
// Example: Route("/user/{id}", handler) matches "/user/123".
func (r *Router) Route(pattern string, handler Handler) {
	pattern = strings.Trim(pattern, Slash)
	var fullPattern string
	if pattern == Default || pattern == Empty {
		fullPattern = r.prefix
		if fullPattern == "" {
			fullPattern = Slash
		}
	} else {
		fullPattern = r.prefix + pattern
	}
	parts := strings.Split(fullPattern, Slash)
	node := r.root
	for _, part := range parts {
		if part == "" {
			continue
		}
		key := part
		isParam := len(part) > 2 && part[0] == '{' && part[len(part)-1] == '}'
		if isParam {
			key = "{param}"
		}
		node = node.AddChild(key, isParam, strings.Trim(part, "{}"))
	}
	node.handler = r.Apply(handler, r.middleware)
}

// Handle processes an incoming message by matching it to a route.
// It executes the handler with middleware, writing errors if any occur.
func (r *Router) Handle(ctx *Frame) error {
	if ctx.Payload == nil {
		return fmt.Errorf("no payload provided")
	}

	frame := r.pool.Get().(*Frame)
	frame.Conn = ctx.Conn
	frame.Values = make(map[string]interface{})
	frame.Errors = nil
	frame.Route = ""
	frame.Payload = ctx.Payload // Use provided Payload

	command := frame.Payload.Command()
	parts := strings.Split(command, Slash)
	if len(parts) > 0 && parts[0] == Empty {
		parts = parts[1:]
	}

	handler, params := r.root.FindHandler(parts)
	if params != nil {
		frame.Params = params
	}

	if handler != nil {
		handler(frame)
		if len(frame.Errors) > 0 {
			if writeErr := r.writeErrors(frame); writeErr != nil {
				frame.Errors = append(frame.Errors, writeErr)
			}
		}
	} else {
		frame.Errors = append(frame.Errors, fmt.Errorf("unknown command: %s", command))
		if writeErr := r.writeErrors(frame); writeErr != nil {
			frame.Errors = append(frame.Errors, writeErr)
		}
	}

	ctx.mu.Lock()
	ctx.Errors = append(ctx.Errors, frame.Errors...)
	ctx.mu.Unlock()

	r.pool.Put(frame)

	if len(ctx.Errors) > 0 {
		return fmt.Errorf("errors occurred during handling: %v", ctx.Errors)
	}
	return nil
}

// writeErrors writes all collected errors to the connection.
// It preserves ctx.Errors for inspection; errors are not cleared.
func (r *Router) writeErrors(ctx *Frame) error {
	if len(ctx.Errors) == 0 {
		return nil
	}
	var errMsg strings.Builder
	for i, err := range ctx.Errors {
		if i >= ErrorLimit { // Limit to 10 errors
			break
		}
		errMsg.WriteString(err.Error() + "\n")
	}
	return ctx.Conn.Write([]byte(errMsg.String()))
}

// Apply wraps a handler with the router’s middleware stack.
// Middleware executes in order, stopping if any returns an error.
func (r *Router) Apply(handler Handler, middleware []func(*Frame) error) Handler {
	return func(ctx *Frame) {
		for _, mw := range middleware {
			if err := mw(ctx); err != nil {
				ctx.Errors = append(ctx.Errors, err)
				return
			}
		}
		handler(ctx)
	}
}

// Routes returns a list of all registered route patterns.
// Example: [Slash, "/user/{id}", "/api/admin"].
func (r *Router) Routes() []string {
	var routes []string
	r.collectRoutes(r.root, Empty, &routes)
	return routes
}

// collectRoutes recursively builds the list of route patterns from the trie.
// It uses a strings.Builder to efficiently construct route paths.
func (r *Router) collectRoutes(node *Node, currentPath string, routes *[]string) {
	if node.handler != nil {
		*routes = append(*routes, currentPath)
	}

	var b strings.Builder
	for key, child := range node.children {
		b.Reset()
		b.WriteString(currentPath)
		if currentPath != Empty && !strings.HasSuffix(currentPath, Slash) {
			b.WriteString(Slash)
		}
		if child.isParam {
			b.WriteString(Open)
			b.WriteString(child.paramName)
			b.WriteString(Close)
		} else {
			b.WriteString(key)
		}
		r.collectRoutes(child, b.String(), routes)
	}
}
