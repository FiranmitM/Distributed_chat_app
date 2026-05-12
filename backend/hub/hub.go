package hub

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/FiranmitM/Distributed_chat_app/models"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
	redisChanPrefix = "chat:room:"
)

// Client represents a connected WebSocket client
type Client struct {
	ID       string
	UserID   string
	Username string
	Avatar   string
	RoomID   string
	conn     *websocket.Conn
	send     chan []byte
	hub      *Hub
}

// Hub manages all clients and rooms, with Redis for distributed pub/sub
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client          // clientID -> Client
	rooms   map[string]map[string]*Client // roomID -> clientID -> Client
	rdb     *redis.Client
	nodeID  string

	// In-memory store (replace with DB in production)
	roomStore    map[string]*models.Room
	messageStore map[string][]*models.Message // roomID -> messages
	userStore    map[string]*models.User       // userID -> User
}

func NewHub(rdb *redis.Client, nodeID string) *Hub {
	h := &Hub{
		clients:      make(map[string]*Client),
		rooms:        make(map[string]map[string]*Client),
		rdb:          rdb,
		nodeID:       nodeID,
		roomStore:    make(map[string]*models.Room),
		messageStore: make(map[string][]*models.Message),
		userStore:    make(map[string]*models.User),
	}
	h.seedRooms()
	return h
}

func (h *Hub) seedRooms() {
	defaults := []models.Room{
		{ID: "general", Name: "# general", Description: "General discussion for everyone", CreatedBy: "system"},
		{ID: "tech", Name: "# tech", Description: "Technology and programming talk", CreatedBy: "system"},
		{ID: "random", Name: "# random", Description: "Off-topic conversations", CreatedBy: "system"},
	}
	for i := range defaults {
		defaults[i].CreatedAt = time.Now()
		h.roomStore[defaults[i].ID] = &defaults[i]
		h.rooms[defaults[i].ID] = make(map[string]*Client)
	}
}

// Run starts the Redis subscriber for distributed messaging
func (h *Hub) Run(ctx context.Context) {
	pubsub := h.rdb.PSubscribe(ctx, redisChanPrefix+"*")
	defer pubsub.Close()

	log.Printf("[%s] Hub running, subscribed to Redis channels", h.nodeID)

	for msg := range pubsub.Channel() {
		roomID := msg.Channel[len(redisChanPrefix):]
		h.broadcastToLocalClients(roomID, []byte(msg.Payload))
	}
}

func (h *Hub) broadcastToLocalClients(roomID string, data []byte) {
	h.mu.RLock()
	clients := h.rooms[roomID]
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			h.removeClient(c)
		}
	}
}

// PublishMessage publishes a message to Redis so all nodes receive it
func (h *Hub) PublishMessage(ctx context.Context, roomID string, msg *models.Message) {
	h.mu.Lock()
	h.messageStore[roomID] = append(h.messageStore[roomID], msg)
	// Keep last 100 messages per room
	if len(h.messageStore[roomID]) > 100 {
		h.messageStore[roomID] = h.messageStore[roomID][len(h.messageStore[roomID])-100:]
	}
	h.mu.Unlock()

	wsMsg := models.WSMessage{Type: "message", Payload: msg, RoomID: roomID}
	data, _ := json.Marshal(wsMsg)
	h.rdb.Publish(ctx, redisChanPrefix+roomID, string(data))
}

func (h *Hub) RegisterClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c.ID] = c
	if _, ok := h.rooms[c.RoomID]; !ok {
		h.rooms[c.RoomID] = make(map[string]*Client)
	}
	h.rooms[c.RoomID][c.ID] = c

	// Update user online status
	if u, ok := h.userStore[c.UserID]; ok {
		u.Online = true
	}

	h.broadcastPresence(c.RoomID, c.UserID, c.Username, "joined")
}

func (h *Hub) removeClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[c.ID]; !ok {
		return
	}
	delete(h.clients, c.ID)
	delete(h.rooms[c.RoomID], c.ID)
	close(c.send)

	if u, ok := h.userStore[c.UserID]; ok {
		u.Online = false
	}

	h.broadcastPresence(c.RoomID, c.UserID, c.Username, "left")
}

func (h *Hub) broadcastPresence(roomID, userID, username, event string) {
	msg := models.WSMessage{
		Type: "presence",
		Payload: map[string]string{
			"user_id":  userID,
			"username": username,
			"event":    event,
		},
		RoomID: roomID,
	}
	data, _ := json.Marshal(msg)
	h.rdb.Publish(context.Background(), redisChanPrefix+roomID, string(data))
}

// GetRooms returns all rooms with live member counts
func (h *Hub) GetRooms() []*models.Room {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rooms := make([]*models.Room, 0, len(h.roomStore))
	for _, r := range h.roomStore {
		r.MemberCount = len(h.rooms[r.ID])
		rooms = append(rooms, r)
	}
	return rooms
}

func (h *Hub) GetRoom(id string) (*models.Room, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.roomStore[id]
	return r, ok
}

func (h *Hub) CreateRoom(room *models.Room) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roomStore[room.ID] = room
	h.rooms[room.ID] = make(map[string]*Client)
}

func (h *Hub) GetMessages(roomID string) []*models.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	msgs := h.messageStore[roomID]
	if msgs == nil {
		return []*models.Message{}
	}
	return msgs
}

func (h *Hub) SaveUser(user *models.User) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.userStore[user.ID] = user
}

func (h *Hub) GetUserByEmail(email string) (*models.User, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, u := range h.userStore {
		if u.Email == email {
			return u, true
		}
	}
	return nil, false
}

func (h *Hub) GetUserByUsername(username string) (*models.User, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, u := range h.userStore {
		if u.Username == username {
			return u, true
		}
	}
	return nil, false
}

// ServeWS upgrades HTTP to WebSocket and starts client read/write pumps
func (h *Hub) ServeWS(conn *websocket.Conn, userID, username, avatar, roomID string) {
	client := &Client{
		ID:       uuid.New().String(),
		UserID:   userID,
		Username: username,
		Avatar:   avatar,
		RoomID:   roomID,
		conn:     conn,
		send:     make(chan []byte, 256),
		hub:      h,
	}

	h.RegisterClient(client)

	// Send message history
	history := h.GetMessages(roomID)
	histMsg := models.WSMessage{Type: "history", Payload: history, RoomID: roomID}
	if data, err := json.Marshal(histMsg); err == nil {
		client.send <- data
	}

	go client.writePump()
	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.removeClient(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var incoming struct {
			Content string `json:"content"`
			Type    string `json:"type"`
		}
		if err := json.Unmarshal(raw, &incoming); err != nil {
			continue
		}

		if strings.TrimSpace(incoming.Content) == "" {
			continue
		}

		msg := &models.Message{
			ID:        uuid.New().String(),
			RoomID:    c.RoomID,
			UserID:    c.UserID,
			Username:  c.Username,
			Avatar:    c.Avatar,
			Content:   incoming.Content,
			Type:      "text",
			CreatedAt: time.Now(),
		}

		c.hub.PublishMessage(context.Background(), c.RoomID, msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
