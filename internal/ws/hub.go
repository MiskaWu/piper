package ws

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const writeWait = 5 * time.Second

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins for local dev
	},
}

// Hub manages a set of WebSocket clients and broadcasts messages to them.
type Hub struct {
	clients    map[*websocket.Conn]bool
	mu         sync.RWMutex
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

// NewHub creates a new Hub with initialized channels and client map.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn, 16),
		unregister: make(chan *websocket.Conn, 16),
	}
}

// Run processes register, unregister, and broadcast events in a loop.
// It should be started as a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			var failed []*websocket.Conn
			for conn := range h.clients {
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("ws: write error: %v", err)
					conn.Close()
					failed = append(failed, conn)
				}
			}
			h.mu.RUnlock()

			if len(failed) > 0 {
				h.mu.Lock()
				for _, conn := range failed {
					delete(h.clients, conn)
				}
				h.mu.Unlock()
			}
		}
	}
}

// Broadcast sends msg to all connected clients.
// Non-blocking: drops the message if the broadcast channel is full.
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		log.Printf("ws: broadcast channel full, dropping message")
	}
}

// ServeHTTP implements http.Handler, delegating to HandleWebSocket.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.HandleWebSocket(w, r)
}

// HandleWebSocket upgrades the HTTP connection to WebSocket and registers it
// with the hub. It blocks until the client disconnects.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade error: %v", err)
		return
	}

	h.register <- conn

	defer func() {
		h.unregister <- conn
		conn.Close()
	}()

	// Read loop: discard messages, detect disconnect.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
