package ruta

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// mockConn is a minimal ConnType for benchmarking
type mockConn struct {
	buf bytes.Buffer
}

func (c *mockConn) Read() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *mockConn) Write(message []byte) error {
	_, err := c.buf.Write(message)
	return err
}

func (c *mockConn) Close() error {
	return nil
}

func (c *mockConn) Reset() {
	c.buf.Reset()
}

// BenchmarkHandle measures the performance of Handle with a simple route
func BenchmarkHandle(b *testing.B) {
	router := NewRouter()
	router.Route("/user/{id}", func(f *Frame) {
		id, _ := f.Params.Get("id")
		f.Conn.Write([]byte("Hello, " + id + "\n"))
	})

	conn := &mockConn{}
	frame := &Frame{
		Conn:    conn,
		Message: []byte("/user/john"),
		Params:  NewParams(),
		Values:  make(map[string]interface{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Reset()
		router.Handle(frame)
	}
}

// BenchmarkHandleWithPool measures Handle with sync.Pool optimization
func BenchmarkHandleWithPool(b *testing.B) {
	router := &Router{
		root: NewNode(),
	}
	router.pool = sync.Pool{
		New: func() interface{} {
			return &Frame{
				Params: NewParams(),
				Values: make(map[string]interface{}),
			}
		},
	}
	router.Route("/user/{id}", func(f *Frame) {
		id, _ := f.Params.Get("id")
		f.Conn.Write([]byte("Hello, " + id + "\n"))
	})

	conn := &mockConn{}
	input := []byte("/user/john")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Reset()
		frame := router.pool.Get().(*Frame)
		frame.Conn = conn
		frame.Message = input
		frame.Errors = nil

		router.Handle(frame)
		router.pool.Put(frame)
	}
}

// BenchmarkRoute measures the performance of route registration
func BenchmarkRoute(b *testing.B) {
	router := NewRouter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(fmt.Sprintf("/path/%d", i), func(f *Frame) {
			f.Conn.Write([]byte("ok\n"))
		})
	}
}

// BenchmarkFindHandler measures the performance of trie lookup
func BenchmarkFindHandler(b *testing.B) {
	node := NewNode()
	n1 := node.AddChild("user", false, "")
	n2 := n1.AddChild("{param}", true, "id")
	n2.handler = func(f *Frame) {
		f.Conn.Write([]byte("mock\n"))
	}
	parts := []string{"user", "john"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.FindHandler(parts)
	}
}

// BenchmarkHandleComplex measures Handle with nested routes and middleware
func BenchmarkHandleComplex(b *testing.B) {
	router := NewRouter()
	router.Use(func(f *Frame) error {
		f.Conn.Write([]byte("mw1\n"))
		return nil
	})
	router.Section("/api", func(r RouteBase) {
		r.Use(func(f *Frame) error {
			f.Conn.Write([]byte("mw2\n"))
			return nil
		})
		r.Route("/user/{id}", func(f *Frame) {
			id, _ := f.Params.Get("id")
			f.Conn.Write([]byte("Hello, " + id + "\n"))
		})
	})

	conn := &mockConn{}
	frame := &Frame{
		Conn:    conn,
		Message: []byte("/api/user/john"),
		Params:  NewParams(),
		Values:  make(map[string]interface{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Reset()
		router.Handle(frame)
	}
}
