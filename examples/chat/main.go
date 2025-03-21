//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"github.com/olekukonko/ruta"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type ChatServer struct {
	clients   map[string]*Client
	broadcast chan Message
	mutex     sync.RWMutex
	router    ruta.RouteBase
}

type Client struct {
	conn     *WSConn
	username string
}

type Message struct {
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Content  string `json:"content,omitempty"`
}

type WSConn struct {
	*websocket.Conn
	mutex sync.Mutex // Add mutex for safe concurrent writes
}

func (w *WSConn) Read() ([]byte, error) {
	_, msg, err := w.ReadMessage()
	return msg, err
}

func (w *WSConn) Write(message []byte) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.WriteMessage(websocket.TextMessage, message)
}

func (w *WSConn) Close() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.Conn.Close()
}

func NewChatServer() *ChatServer {
	cs := &ChatServer{
		clients:   make(map[string]*Client),
		broadcast: make(chan Message),
		router:    ruta.NewRouter(),
	}

	cs.setupRoutes()
	go cs.handleMessages()
	return cs
}

func (cs *ChatServer) setupRoutes() {
	usernameCheck := func(f *ruta.Frame) error {
		payload, err := f.Payload.Text()
		if err != nil {
			return err
		}
		parts := strings.SplitN(payload, " ", 2)
		if len(parts) < 1 {
			return fmt.Errorf("username required in payload")
		}
		username := parts[0]
		cs.mutex.RLock()
		_, exists := cs.clients[username]
		cs.mutex.RUnlock()
		if !exists {
			return fmt.Errorf("unknown user: %s", username)
		}
		f.Values["username"] = username
		if len(parts) > 1 {
			f.Values["content"] = parts[1]
		} else {
			f.Values["content"] = ""
		}
		return nil
	}

	cs.router.Route("/join", func(f *ruta.Frame) {
		var payload struct {
			Username string `json:"username"`
		}
		if err := f.Payload.JSON(&payload); err != nil {
			payloadStr, err := f.Payload.Text()
			if err != nil {
				f.Errors = append(f.Errors, fmt.Errorf("invalid payload: %v", err))
				return
			}
			parts := strings.SplitN(payloadStr, ":", 2)
			if len(parts) < 2 || parts[0] != "username" {
				f.Errors = append(f.Errors, fmt.Errorf("invalid payload: expect username:<name> or JSON"))
				return
			}
			payload.Username = parts[1]
		}

		cs.mutex.Lock()
		if _, exists := cs.clients[payload.Username]; exists {
			f.Errors = append(f.Errors, fmt.Errorf("username %s already taken", payload.Username))
			cs.mutex.Unlock()
			return
		}
		cs.clients[payload.Username] = &Client{conn: f.Conn.(*WSConn), username: payload.Username}
		cs.mutex.Unlock()

		log.Printf("User %s joined", payload.Username)

		resp := Message{Type: "system", Content: fmt.Sprintf("joined as %s", payload.Username)}
		if jsonData, err := json.Marshal(resp); err == nil {
			f.Conn.Write(jsonData)
		}

		cs.broadcast <- Message{
			Type:    "system",
			Content: fmt.Sprintf("%s has joined the chat", payload.Username),
		}
	})

	cs.router.Section("/mess", func(r ruta.RouteBase) {
		r.Use(usernameCheck)
		r.Route("/send", func(f *ruta.Frame) {
			username := f.Values["username"].(string)
			content, ok := f.Values["content"].(string)
			if !ok || content == "" {
				f.Errors = append(f.Errors, fmt.Errorf("message content required"))
				return
			}

			cs.mutex.RLock()
			client := cs.clients[username]
			cs.mutex.RUnlock()

			log.Printf("Message from %s: %s", client.username, content)

			cs.broadcast <- Message{
				Type:     "message",
				Username: client.username,
				Content:  content,
			}

			resp := Message{Type: "system", Content: "message sent"}
			if jsonData, err := json.Marshal(resp); err == nil {
				f.Conn.Write(jsonData)
			}
		})
	})

	cs.router.Route("/users", func(f *ruta.Frame) {
		cs.mutex.RLock()
		var users []string
		for username := range cs.clients {
			users = append(users, username)
		}
		cs.mutex.RUnlock()

		log.Printf("Users requested: %v", users)

		resp := Message{
			Type:    "users",
			Content: fmt.Sprintf("Online users: %s", strings.Join(users, ", ")),
		}
		if jsonData, err := json.Marshal(resp); err == nil {
			f.Conn.Write(jsonData)
		}
	})
}

func (cs *ChatServer) handleMessages() {
	for msg := range cs.broadcast {
		cs.mutex.RLock()
		jsonData, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Error marshaling broadcast message: %v", err)
			cs.mutex.RUnlock()
			continue
		}

		for _, client := range cs.clients {
			if err := client.conn.Write(jsonData); err != nil {
				log.Printf("Error broadcasting to %v: %v", client.username, err)
			}
		}
		cs.mutex.RUnlock()
	}
}

func (cs *ChatServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	wsConn := &WSConn{Conn: conn}

	conn.WriteJSON(Message{
		Type:    "system",
		Content: "Connected. Please set your username",
	})

	go func() {
		defer func() {
			cs.mutex.Lock()
			var username string
			for u, c := range cs.clients {
				if c.conn == wsConn {
					username = u
					delete(cs.clients, u)
					break
				}
			}
			wsConn.Close()
			log.Printf("Connection closed for %s", username)
			cs.mutex.Unlock()

			if username != "" {
				cs.broadcast <- Message{
					Type:    "system",
					Content: fmt.Sprintf("%s has left the chat", username),
				}
			}
		}()

		for {
			_, rawMessage, err := conn.ReadMessage()
			if err != nil {
				return
			}

			log.Printf("Received raw message: %s", string(rawMessage))

			frame := &ruta.Frame{
				Conn:    wsConn,
				Payload: ruta.NewPayload(rawMessage),
			}
			if err := cs.router.Handle(frame); err != nil {
				log.Printf("Error handling message: %v", err)
			}
		}
	}()
}

func main() {
	chatServer := NewChatServer()

	http.HandleFunc("/ws", chatServer.handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func LoggingMiddleware(frame *ruta.Frame) error {
	log.Printf("log command: %s", frame.Command())
	return nil
}
