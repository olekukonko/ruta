package ruta

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// MockConn is a mock implementation of ConnType for testing
type MockConn struct {
	Buffer bytes.Buffer
	Closed bool
}

func (c *MockConn) Read() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *MockConn) Write(message []byte) error {
	if c.Closed {
		return fmt.Errorf("connection closed")
	}
	_, err := c.Buffer.Write(message)
	return err
}

func (c *MockConn) Close() error {
	c.Closed = true
	return nil
}

// Reset resets the MockConn for reuse
func (c *MockConn) Reset() {
	c.Buffer.Reset()
	c.Closed = false
}

// MockHandler is a simple handler for testing
func mockHandler(ctx *Frame) {
	ctx.Conn.Write([]byte("mock\n"))
}

// TestParams tests the Params type
func TestParams(t *testing.T) {
	tests := []struct {
		name    string
		ops     func(*Params) // Operations to perform
		key     string        // Key to get
		wantVal string        // Expected value
		wantOK  bool          // Expected ok result
	}{
		{
			name: "Set and get",
			ops: func(p *Params) {
				p.Set("key", "value")
			},
			key:     "key",
			wantVal: "value",
			wantOK:  true,
		},
		{
			name:    "Get non-existent",
			ops:     func(p *Params) {},
			key:     "missing",
			wantVal: "",
			wantOK:  false,
		},
		{
			name: "Overwrite value",
			ops: func(p *Params) {
				p.Set("key", "first")
				p.Set("key", "second")
			},
			key:     "key",
			wantVal: "second",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParams()
			tt.ops(p)
			gotVal, gotOK := p.Get(tt.key)
			if gotVal != tt.wantVal || gotOK != tt.wantOK {
				t.Errorf("Get(%q) = %q, %v; want %q, %v", tt.key, gotVal, gotOK, tt.wantVal, tt.wantOK)
			}
		})
	}
}

// TestParamsConcurrency tests concurrent access to Params
func TestParamsConcurrency(t *testing.T) {
	p := NewParams()
	var wg sync.WaitGroup

	// Concurrently set values
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key%d", i)
			p.Set(key, fmt.Sprintf("value%d", i))
		}(i)
	}

	wg.Wait()

	// Verify all values
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key%d", i)
		wantVal := fmt.Sprintf("value%d", i)
		gotVal, ok := p.Get(key)
		if !ok || gotVal != wantVal {
			t.Errorf("Get(%q) = %q, %v; want %q, true", key, gotVal, ok, wantVal)
		}
	}
}

// TestNode tests the Node type
func TestNode(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Node)       // Setup trie structure
		parts   []string          // Path parts to match
		wantOut string            // Expected output from handler
		wantP   map[string]string // Expected params
	}{
		{
			name: "Simple path",
			setup: func(n *Node) {
				n.AddChild("hello", false, "").handler = mockHandler
			},
			parts:   []string{"hello"},
			wantOut: "mock\n",
			wantP:   map[string]string{},
		},
		{
			name: "Nested path",
			setup: func(n *Node) {
				n1 := n.AddChild("service", false, "")
				n1.AddChild("admin", false, "").handler = mockHandler
			},
			parts:   []string{"service", "admin"},
			wantOut: "mock\n",
			wantP:   map[string]string{},
		},
		{
			name: "Path with param",
			setup: func(n *Node) {
				n1 := n.AddChild("user", false, "")
				n1.AddChild("{param}", true, "username").handler = mockHandler
			},
			parts:   []string{"user", "john"},
			wantOut: "mock\n",
			wantP:   map[string]string{"username": "john"},
		},
		{
			name: "Trailing slash",
			setup: func(n *Node) {
				n.AddChild("service", false, "").handler = mockHandler
			},
			parts:   []string{"service", ""},
			wantOut: "mock\n",
			wantP:   map[string]string{},
		},
		{
			name: "No match",
			setup: func(n *Node) {
				n.AddChild("hello", false, "").handler = mockHandler
			},
			parts:   []string{"goodbye"},
			wantOut: "",
			wantP:   nil,
		},
		{
			name: "Partial match",
			setup: func(n *Node) {
				n1 := n.AddChild("service", false, "")
				n1.AddChild("admin", false, "").handler = mockHandler
			},
			parts:   []string{"service"},
			wantOut: "",
			wantP:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNode()
			tt.setup(n)

			gotH, gotP := n.FindHandler(tt.parts)

			// Check handler by executing it
			conn := &MockConn{}
			ctx := &Frame{
				Conn:    conn,
				Payload: NewPayload([]byte(strings.Join(tt.parts, "/"))), // Use Payload instead of Message
				Params:  NewParams(),
				Values:  make(map[string]interface{}),
			}
			if gotH != nil {
				if gotP != nil {
					ctx.Params = gotP
				}
				gotH(ctx)
			}
			gotOut := conn.Buffer.String()
			if gotOut != tt.wantOut {
				t.Errorf("FindHandler() output = %q, want %q", gotOut, tt.wantOut)
			}

			// Check params
			if gotP == nil && tt.wantP == nil {
				return
			}
			if gotP == nil || tt.wantP == nil {
				t.Errorf("FindHandler() params = %v, want %v", gotP, tt.wantP)
				return
			}

			wantMap := tt.wantP
			gotMap := make(map[string]string)
			gotP.data.Range(func(key, value interface{}) bool {
				gotMap[key.(string)] = value.(string)
				return true
			})
			if !reflect.DeepEqual(gotMap, wantMap) {
				t.Errorf("FindHandler() params = %v, want %v", gotMap, wantMap)
			}
		})
	}
}

// TestNodeAddChild tests the AddChild method
func TestNodeAddChild(t *testing.T) {
	n := NewNode()

	// Add a regular child
	child1 := n.AddChild("test", false, "")
	if child1 == nil || child1 != n.children["test"] {
		t.Errorf("AddChild(test) did not add child correctly")
	}
	if child1.isParam || child1.paramName != "" {
		t.Errorf("AddChild(test) set unexpected param properties: isParam=%v, paramName=%q", child1.isParam, child1.paramName)
	}

	// Add a param child
	child2 := n.AddChild("{param}", true, "id")
	if child2 == nil || child2 != n.children["{param}"] {
		t.Errorf("AddChild({param}) did not add child correctly")
	}
	if !child2.isParam || child2.paramName != "id" {
		t.Errorf("AddChild({param}) param properties: isParam=%v, paramName=%q; want true, id", child2.isParam, child2.paramName)
	}

	// Add duplicate child
	child3 := n.AddChild("test", false, "")
	if child3 != child1 {
		t.Errorf("AddChild(test) on duplicate returned new node, want existing")
	}
}

// TestRouter tests the RouteBase implementation
func TestRouter(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(RouteBase) // Setup routes
		input   string          // Input message to Handle
		wantOut string          // Expected output
		wantErr bool            // Expect an error
		wantRts []string        // Expected registered routes
	}{
		{
			name: "Simple route",
			setup: func(r RouteBase) {
				r.Route("hello", func(ctx *Frame) {
					ctx.Conn.Write([]byte("hi\n"))
				})
			},
			input:   "hello",
			wantOut: "hi\n",
			wantRts: []string{"hello"},
		},
		{
			name: "Route with param",
			setup: func(r RouteBase) {
				r.Route("/user/{username}", func(ctx *Frame) {
					username, _ := ctx.Params.Get("username")
					ctx.Conn.Write([]byte("Hello, " + username + "\n"))
				})
			},
			input:   "/user/john",
			wantOut: "Hello, john\n",
			wantRts: []string{"user/{username}"},
		},
		{
			name: "Unknown command",
			setup: func(r RouteBase) {
				r.Route("hello", func(ctx *Frame) {
					ctx.Conn.Write([]byte("hi\n"))
				})
			},
			input:   "goodbye",
			wantOut: "unknown command: goodbye\n",
			wantErr: true,
			wantRts: []string{"hello"},
		},
		{
			name: "Middleware",
			setup: func(r RouteBase) {
				r.Use(func(ctx *Frame) error {
					ctx.Conn.Write([]byte("Middleware\n"))
					return nil
				})
				r.Route("test", func(ctx *Frame) {
					ctx.Conn.Write([]byte("Handler\n"))
				})
			},
			input:   "test",
			wantOut: "Middleware\nHandler\n",
			wantRts: []string{"test"},
		},
		{
			name: "Nested section with default",
			setup: func(r RouteBase) {
				r.Section("/service", func(r RouteBase) {
					r.Use(func(ctx *Frame) error {
						ctx.Conn.Write([]byte("Service Use\n"))
						return nil
					})
					r.Route("default", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service base\n"))
					})
					r.Route("/a", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service a\n"))
					})
				})
			},
			input:   "/service",
			wantOut: "Service Use\nservice base\n",
			wantRts: []string{"service", "service/a"},
		},
		{
			name: "Nested section with /",
			setup: func(r RouteBase) {
				r.Section("/service", func(r RouteBase) {
					r.Use(func(ctx *Frame) error {
						ctx.Conn.Write([]byte("Service Use\n"))
						return nil
					})
					r.Route("/", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service base\n"))
					})
					r.Route("/a", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service a\n"))
					})
				})
			},
			input:   "/service/",
			wantOut: "Service Use\nservice base\n",
			wantRts: []string{"service", "service/a"},
		},
		{
			name: "Nested section with empty",
			setup: func(r RouteBase) {
				r.Section("/service", func(r RouteBase) {
					r.Use(func(ctx *Frame) error {
						ctx.Conn.Write([]byte("Service Use\n"))
						return nil
					})
					r.Route("", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service base\n"))
					})
					r.Route("/a", func(ctx *Frame) {
						ctx.Conn.Write([]byte("service a\n"))
					})
				})
			},
			input:   "/service",
			wantOut: "Service Use\nservice base\n",
			wantRts: []string{"service", "service/a"},
		},
		{
			name: "Deep nesting with group",
			setup: func(r RouteBase) {
				r.Section("/service", func(r RouteBase) {
					r.Use(func(ctx *Frame) error {
						ctx.Conn.Write([]byte("Service Use\n"))
						return nil
					})
					r.Section("/admin", func(r RouteBase) {
						r.Route("default", func(ctx *Frame) {
							ctx.Conn.Write([]byte("admin base\n"))
						})
						r.Group(func(r RouteBase) {
							r.Use(func(ctx *Frame) error {
								ctx.Conn.Write([]byte("Admin Group Use\n"))
								return nil
							})
							r.Route("/yes", func(ctx *Frame) {
								ctx.Conn.Write([]byte("admin yes\n"))
							})
						})
					})
				})
			},
			input:   "/service/admin/yes",
			wantOut: "Service Use\nAdmin Group Use\nadmin yes\n",
			wantRts: []string{"service/admin", "service/admin/yes"},
		},
		{
			name: "No payload",
			setup: func(r RouteBase) {
				r.Route("hello", func(ctx *Frame) {
					ctx.Conn.Write([]byte("hi\n"))
				})
			},
			input:   "", // Will trigger "no payload" error
			wantOut: "",
			wantErr: true,
			wantRts: []string{"hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup router
			router := NewRouter()
			tt.setup(router)

			// Test registered routes
			gotRoutes := router.Routes()
			sort.Strings(gotRoutes) // Sort for consistent comparison
			sort.Strings(tt.wantRts)
			if !reflect.DeepEqual(gotRoutes, tt.wantRts) {
				t.Errorf("Routes() = %v, want %v", gotRoutes, tt.wantRts)
			}

			// Test route handling
			conn := &MockConn{}
			var ctx *Frame
			if tt.input != "" {
				ctx = &Frame{
					Conn:    conn,
					Payload: NewPayload([]byte(tt.input)), // Use Payload instead of Message
					Params:  NewParams(),
					Values:  make(map[string]interface{}),
				}
			} else {
				ctx = &Frame{
					Conn:   conn,
					Params: NewParams(),
					Values: make(map[string]interface{}),
				}
			}

			err := router.Handle(ctx)

			// Check output
			gotOut := conn.Buffer.String()
			if gotOut != tt.wantOut {
				t.Errorf("Handle() output = %q, want %q", gotOut, tt.wantOut)
			}

			// Check errors
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("Handle() error = %v, wantErr %v", err, tt.wantErr)
			}

			conn.Reset()
		})
	}
}

// TestConcurrentRouteHandling tests concurrent access to the router
func TestConcurrentRouteHandling(t *testing.T) {
	router := NewRouter()
	router.Use(func(ctx *Frame) error {
		ctx.Conn.Write([]byte("Middleware\n"))
		return nil
	})
	router.Route("/test", func(ctx *Frame) {
		ctx.Conn.Write([]byte("Handler\n"))
	})

	var wg sync.WaitGroup
	conns := make([]*MockConn, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		conns[i] = &MockConn{}
		go func(conn *MockConn, i int) {
			defer wg.Done()
			ctx := &Frame{
				Conn:    conn,
				Payload: NewPayload([]byte("/test")), // Use Payload instead of Message
				Params:  NewParams(),
				Values:  make(map[string]interface{}),
			}
			err := router.Handle(ctx)
			if err != nil {
				t.Errorf("Handle() error in goroutine %d: %v", i, err)
			}
		}(conns[i], i)
	}

	wg.Wait()

	for i, conn := range conns {
		gotOut := conn.Buffer.String()
		wantOut := "Middleware\nHandler\n"
		if gotOut != wantOut {
			t.Errorf("Conn %d: Handle() output = %q, want %q", i, gotOut, wantOut)
		}
	}
}
