# Ruta

**A lightweight, flexible router for TCP and WebSocket applications in Go.**

[![GoDoc](https://godoc.org/github.com/olekukonko/ruta?status.svg)](https://godoc.org/github.com/olekukonko/ruta) [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Ruta is a Chi-inspired routing library tailored for network protocols like TCP and WebSocket, where traditional HTTP routers fall short. While HTTP routing libraries abound, options for TCP and WebSocket are scarce—most implementations resort to cumbersome switch statements or brittle string parsing. Ruta fills this gap with a clean, trie-based routing system that’s fast, extensible, and easy to use.

Whether you’re building a chat server, a real-time game, or a custom protocol over TCP, Ruta provides a robust foundation for handling commands and messages with the elegance of a modern router.

---

## Why Ruta?

- **Protocol-Agnostic**: Works seamlessly with TCP, WebSocket, or any custom protocol that implements the `Connection` interface.
- **Trie-Based Efficiency**: Uses a prefix tree (trie) for fast, scalable route matching—no more nested `if`/`switch` messes.
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
