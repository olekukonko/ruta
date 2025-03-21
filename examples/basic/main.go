//go:build ignore
// +build ignore

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
	server := NewServer(router)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	// Start TCP and WebSocket servers
	go handleTCP(wg, server)
	go handleWS(wg, server)

	// Wait for servers to be ready
	wg.Wait()

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
