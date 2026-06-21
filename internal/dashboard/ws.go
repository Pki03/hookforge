package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prateekkhurmi/hookforge/internal/database"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHandler struct {
	db      *database.DB
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewWSHandler(db *database.DB) *WSHandler {
	return &WSHandler{
		db:      db,
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *WSHandler) Serve(c http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(c, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats, err := h.db.GetStats(r.Context())
		if err != nil {
			continue
		}

		events, err := h.db.ListEvents(r.Context(), "", 10)
		if err != nil {
			continue
		}

		msg, _ := json.Marshal(map[string]interface{}{
			"stats":  stats,
			"events": events,
		})

		h.mu.RLock()
		for client := range h.clients {
			client.SetWriteDeadline(time.Now().Add(3 * time.Second))
			if err := client.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("ws write: %v", err)
				client.Close()
				go func(c *websocket.Conn) {
					h.mu.Lock()
					delete(h.clients, c)
					h.mu.Unlock()
				}(client)
			}
		}
		h.mu.RUnlock()
	}
}
