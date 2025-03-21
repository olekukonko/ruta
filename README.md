# Ruta

**A lightweight, flexible router for TCP and WebSocket applications in Go.**

[![GoDoc](https://godoc.org/github.com/olekukonko/ruta?status.svg)](https://godoc.org/github.com/olekukonko/ruta) [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Ruta is a Chi-inspired routing library tailored for network protocols like TCP and WebSocket, where traditional HTTP routers fall short. While HTTP routing libraries abound, options for TCP and WebSocket are scarce, most implementations resort to cumbersome switch statements or brittle string parsing. Ruta fills this gap with a clean, trie-based routing system that’s fast, extensible, and easy to use.

Whether you’re building a chat server, a real-time game, or a custom protocol over TCP, Ruta provides a robust foundation for handling commands and messages with the elegance of a modern router.

---

## Why Ruta?

- **Protocol-Agnostic**: Works seamlessly with TCP, WebSocket, or any custom protocol that implements the `Connection` interface.
- **Trie-Based Efficiency**: Uses a prefix tree (trie) for fast, scalable route matching - no more nested `if`/`switch` messes.
- **Chi-Inspired Design**: Borrows the best ideas from Chi (middleware, route grouping, parameter parsing) and adapts them for non-HTTP use cases.
- **Lightweight**: Zero external dependencies beyond the Go standard library.
- **Flexible Middleware**: Stack middleware globally, per route, or per group to handle authentication, logging, or custom logic.
- **Parameterized Routes**: Easily extract dynamic values from commands (e.g., `/user/{id}`).
- **Thread-Safe**: Built-in concurrency support for parameters and error handling.

Ruta is perfect for developers who want a structured, maintainable way to route messages in real-time applications without reinventing the wheel.

---

## Features

- **Route Registration**: Define routes with static paths (e.g., `/join`) or parameters (e.g., `/user/{username}`).
- **Middleware Support**: Apply middleware at the router, section, or group level for pre- and post-processing.
- **Route Sections**: Group related routes under prefixes (e.g., `/mess/send`, `/mess/list`).
- **Parameter Extraction**: Access route parameters safely and concurrently with `Params`.
- **Error Handling**: Collect and propagate errors through a thread-safe `Frame`.
- **Route Listing**: Inspect registered routes with `Routes()` for debugging or documentation.
- **Pooling**: Reuses `Frame` objects to minimize memory allocations in high-throughput scenarios.

---

## Installation

Install Ruta using `go get`:

```bash
go get github.com/olekukonko/ruta
```

Ruta has no external dependencies, so it’s ready to use out of the box with Go 1.13+.

---

## Quick Start

Here’s a simple example of a WebSocket chat server using Ruta with the `gorilla/websocket` package:

```go
package main

import (
	"log"
	"net/http"
	"github.com/olekukonko/ruta"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

// WSConn wraps a WebSocket connection
type WSConn struct{ *websocket.Conn }

func (w WSConn) Read() ([]byte, error) {
	_, msg, err := w.ReadMessage()
	return msg, err
}

func (w WSConn) Write(message []byte) error {
	return w.WriteMessage(websocket.TextMessage, message)
}

func (w WSConn) Close() error { return w.Conn.Close() }

func main() {
	router := ruta.NewRouter()

	// Define a route for joining the chat
	router.Route("/join", func(f *ruta.Frame) {
		payload, _ := f.Payload.Text()
		f.Conn.Write([]byte("Welcome, " + payload + "!\n"))
	})

	// Define a section for message commands
	router.Section("/mess", func(r ruta.RouteBase) {
		r.Route("/send", func(f *ruta.Frame) {
			msg, _ := f.Payload.Text()
			f.Conn.Write([]byte("Sent: " + msg + "\n"))
		})
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		wsConn := &WSConn{conn}

		go func() {
			defer wsConn.Close()
			for {
				_, rawMsg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				frame := &ruta.Frame{
					Conn:    wsConn,
					Payload: ruta.NewPayload(rawMsg),
				}
				if err := router.Handle(frame); err != nil {
					log.Printf("Handle error: %v", err)
				}
			}
		}()
	})

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### Example Interaction
Connect via a WebSocket client (e.g., `wscat`):
```bash
$ wscat -c ws://localhost:8080/ws
> /join alice
< Welcome, alice!
> /mess/send hello world
< Sent: hello world
```

---

## Usage

### Defining Routes
Routes are registered with a pattern and a handler function:

```go
router := ruta.NewRouter()

router.Route("/status", func(f *ruta.Frame) {
    f.Conn.Write([]byte("OK\n"))
})
```

### Parameterized Routes
Capture dynamic segments with `{paramName}`:

```go
router.Route("/user/{id}", func(f *ruta.Frame) {
    id, _ := f.Params.Get("id")
    f.Conn.Write([]byte("User ID: " + id + "\n"))
})
```

### Middleware
Add middleware globally or to specific sections:

```go
router.Use(func(f *ruta.Frame) error {
    log.Printf("Command: %s", f.Command())
    return nil
})

router.Section("/admin", func(r ruta.RouteBase) {
    r.Use(func(f *ruta.Frame) error {
        return fmt.Errorf("admin access required")
    })

    r.Route("/shutdown", func(f *ruta.Frame) {
        f.Conn.Write([]byte("Shutting down\n"))
    })
})
```

### Route Groups
Scope middleware to a subset of routes:

```go
router.Group(func(r ruta.RouteBase) {
    r.Use(func(f *ruta.Frame) error {
        f.Conn.Write([]byte("Group middleware\n"))
        return nil
    })
	
    r.Route("/test1", func(f *ruta.Frame) {})
    r.Route("/test2", func(f *ruta.Frame) {})
})
```

### Handling Payloads
Access the message payload as text, JSON, or raw bytes:

```go
router.Route("/data", func(f *ruta.Frame) {
    // As text
    text, _ := f.Payload.Text()
    f.Conn.Write([]byte("Received: " + text + "\n"))

    // As JSON
    var data struct{ Name string }
    f.Payload.JSON(&data)
})
```

---

## Advanced Usage

### Handling Multiple Protocols

Ruta can handle multiple protocols simultaneously. Below is an example of handling both TCP and WebSocket connections:

```go
package main

import (
	"fmt"
	"github.com/gobwas/ws"
	"github.com/olekukonko/ruta"
	"log"
	"net"
	"sync"
)

func main() {
	// Create a router
	router := ruta.NewRouter()
	router.Use(func(ctx *ruta.Frame) error {
		ctx.Conn.Write([]byte("Default Use\n"))
		return nil
	})

	router.Route("hello", func(ctx *ruta.Frame) {
		ctx.Conn.Write([]byte("hi\n"))
	})

	router.Route("say/hello", func(ctx *ruta.Frame) {
		ctx.Conn.Write([]byte("hello\n"))
	})

	router.Route("/user/{username}", func(ctx *ruta.Frame) {
		username, _ := ctx.Params.Get("username")
		ctx.Conn.Write([]byte("Hello, " + username + "\n"))
	})

	// First /welcome section
	router.Section("/welcome", func(r ruta.RouteBase) {
		r.Route("/message", func(ctx *ruta.Frame) {
			ctx.Conn.Write([]byte("Hello router a\n"))
		})
	})

	// Second /welcome section with group and nested admin section
	router.Section("/service", func(x ruta.RouteBase) {
		x.Group(func(r ruta.RouteBase) {
			r.Use(func(ctx *ruta.Frame) error {
				ctx.Conn.Write([]byte("Group Use\n"))
				return nil
			})

			r.Route("/a", func(ctx *ruta.Frame) {
				ctx.Conn.Write([]byte("service a\n"))
			})

			r.Route("/b", func(ctx *ruta.Frame) {
				ctx.Conn.Write([]byte("service b\n"))
			})

			// Nested admin section
			r.Section("/admin", func(r ruta.RouteBase) {
				r.Route("/", func(ctx *ruta.Frame) { // Default "/" route for /welcome/admin
					ctx.Conn.Write([]byte("admin base\n"))
				})

				r.Group(func(r ruta.RouteBase) {
					r.Use(func(ctx *ruta.Frame) error {
						ctx.Conn.Write([]byte("Sub Group Use\n"))
						return nil
					})

					r.Route("/yes", func(ctx *ruta.Frame) {
						ctx.Conn.Write([]byte("admin yes\n"))
					})

					r.Route("/done", func(ctx *ruta.Frame) {
						ctx.Conn.Write([]byte("admin done\n"))
					})
				})
			})
		})

		x.Route("/done", func(ctx *ruta.Frame) {
			ctx.Conn.Write([]byte("am done a\n"))
		})
	})

	fmt.Println("Registered routes:")
	for _, route := range router.Routes() {
		fmt.Println(route)
	}

	// Create a server with the router
	server := ruta.NewServer(router)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	// Start TCP and WebSocket servers
	go handleTCP(wg, server)
	go handleWS(wg, server)

	// Wait for servers to be ready
	wg.Wait()

	server.Serve()
}

func handleWS(wg *sync.WaitGroup, s *Server) {

	// Create a WebSocket server
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	fmt.Println("WebSocket server is running on :8080")
	wg.Done()

	for {
		// Accept a new connection
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		// Upgrade the connection to a WebSocket connection
		_, err = ws.Upgrade(conn)
		if err != nil {
			log.Printf("Failed to upgrade connection: %v", err)
			conn.Close()
			continue
		}

		// Handle the WebSocket connection
		s.AddConnection(&GobwasConn{conn: conn})
	}
}

func handleTCP(wg *sync.WaitGroup, s *Server) {
	ln, err := net.Listen("tcp", ":8081")
	if err != nil {
		fmt.Printf("Failed to start TCP server: %v\n", err)
		return
	}
	defer ln.Close()

	fmt.Println("TCP server is running on :8081")
	wg.Done()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("Failed to accept TCP connection: %v\n", err)
			continue
		}

		log.Println("TCP Ready for connection")
		s.AddConnection(&TCPConn{conn: conn})
	}
}
```

### Server Implementation

The `Server` struct manages connections and delegates message handling to the router:

```go
package main

import (
	"fmt"
	"github.com/olekukonko/ruta"
	"sync"
)

// Server handles connection management
type Server struct {
	router ruta.RouteBase
	conns  []ruta.Connection // Keep this for reference, but not used in Serve()
	connCh chan ruta.Connection
	mu     sync.Mutex
}

// NewServer creates a new Server instance
func NewServer(router ruta.RouteBase) *Server {
	return &Server{
		router: router,
		conns:  make([]ruta.Connection, 0),
		connCh: make(chan ruta.Connection, 10), // Buffered channel to avoid blocking
	}
}

// AddConnection adds a connection to the server
func (s *Server) AddConnection(conn ruta.Connection) {
	s.mu.Lock()
	s.conns = append(s.conns, conn) // Optional: keep track of connections
	s.mu.Unlock()
	s.connCh <- conn // Send to channel for processing
}

// Serve starts handling connections in a non-blocking way
func (s *Server) Serve() {
	fmt.Printf("Serve was called\n")

	for conn := range s.connCh {
		go func(c ruta.Connection) {
			defer c.Close()

			for {
				msg, err := c.Read()
				if err != nil {
					fmt.Printf("Read error: %v\n", err)
					return
				}

				// Log the received message
				fmt.Printf("Received message: %s\n", string(msg))

				// Create a context and delegate to the router
				ctx := &ruta.Frame{
					Conn:    c,
					Payload: ruta.NewPayload(msg),
					Params:  ruta.NewParams(),
					Values:  make(map[string]interface{}),
				}

				// Let the router handle the message
				s.router.Handle(ctx)
			}
		}(conn)
	}
}
```

### Wrapping Multiple Protocols

Ruta can handle multiple protocols simultaneously. Below is an example of handling both TCP and WebSocket connections. The `TCPConn` and `GobwasConn` structs wrap TCP and WebSocket connections, respectively, to implement the `Connection` interface required by Ruta.

```go
package main

import (
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/gorilla/websocket"
	"log"
	"net"
)

// TCPConn wraps a net.Conn for raw TCP connections
type TCPConn struct {
	conn net.Conn
}

// Read reads data from the TCP connection
func (c *TCPConn) Read() ([]byte, error) {
	buf := make([]byte, 1024)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// Write writes data to the TCP connection
func (c *TCPConn) Write(message []byte) error {
	fmt.Printf("[tcp] %s\n", message)
	_, err := c.conn.Write(message)
	return err
}

// Close closes the TCP connection
func (c *TCPConn) Close() error {
	return c.conn.Close()
}

// GobwasConn wraps gobwas/ws connection
type GobwasConn struct {
	conn net.Conn
}

// Read reads a WebSocket message from the connection
func (c *GobwasConn) Read() ([]byte, error) {
	msg, _, err := wsutil.ReadClientData(c.conn)
	if err != nil {
		log.Printf("WebSocket read error: %v", err)
		return nil, err
	}
	log.Printf("WebSocket read: %s", msg)
	return msg, nil
}

// Write writes a WebSocket message to the connection
func (c *GobwasConn) Write(message []byte) error {
	fmt.Printf("[ws] %s\n", message)
	err := wsutil.WriteServerMessage(c.conn, ws.OpText, message)
	if err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
	return err
}

// Close closes the WebSocket connection
func (c *GobwasConn) Close() error {
	return c.conn.Close()
}
```

---

## Comparison to Alternatives

- **Switch Statements**: Simple but unscalable and error-prone for complex protocols.
- **HTTP Routers (e.g., Chi, Gorilla Mux)**: Great for HTTP, but overkill or incompatible with TCP/WebSocket message streams.
- **Custom Parsers**: Flexible but time-consuming to build and maintain.

Ruta strikes a balance: it’s purpose-built for TCP/WebSocket, lightweight, and feature-rich without HTTP baggage.


Run tests with:
```bash
go test -v
```

---

## License

Ruta is licensed under the [MIT License](LICENSE). Use it freely in your projects!

---

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on [GitHub](https://github.com/olekukonko/ruta).

---

## Acknowledgements

Ruta is inspired by the simplicity and elegance of [Chi](https://github.com/go-chi/chi), a popular HTTP router for Go. Special thanks to the Go community for their invaluable contributions and support.

---

## Roadmap

- **Enhanced Middleware Support**: Add more built-in middleware for common tasks like logging, rate limiting, and authentication.
- **WebSocket Subprotocols**: Add support for WebSocket subprotocols.
- **Benchmarks**: Provide benchmarks to compare Ruta with other routing solutions.
- **Documentation**: Expand documentation with more examples and tutorials.

---

## Support

If you find Ruta useful, consider starring the project on [GitHub](https://github.com/olekukonko/ruta) and sharing it with your network. Your support helps us improve and maintain the library!

---