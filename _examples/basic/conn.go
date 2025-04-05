package main

import (
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/gorilla/websocket"
	"log"
	"net"
)

// GorillaConn wraps gorilla/websocket.Conn
type GorillaConn struct {
	*websocket.Conn
}

func (c *GorillaConn) Read() ([]byte, error) {
	_, msg, err := c.Conn.ReadMessage()
	return msg, err
}

func (c *GorillaConn) Write(message []byte) error {
	fmt.Printf("[gorilla] %s\n", message)
	return c.Conn.WriteMessage(websocket.TextMessage, message)
}

func (c *GorillaConn) Close() error {
	return c.Conn.Close()
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
