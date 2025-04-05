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
