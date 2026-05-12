package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/FiranmitM/Distributed_chat_app/hub"
	"github.com/FiranmitM/Distributed_chat_app/middleware"
	"github.com/FiranmitM/Distributed_chat_app/models"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Handler struct {
	hub *hub.Hub
}

func New(h *hub.Hub) *Handler {
	return &Handler{hub: h}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, "username, email and password required", http.StatusBadRequest)
		return
	}

	if _, exists := h.hub.GetUserByEmail(req.Email); exists {
		writeError(w, "email already registered", http.StatusConflict)
		return
	}
	if _, exists := h.hub.GetUserByUsername(req.Username); exists {
		writeError(w, "username taken", http.StatusConflict)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, "server error", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Avatar:       fmt.Sprintf("https://api.dicebear.com/8.x/avataaars/svg?seed=%s", req.Username),
		CreatedAt:    time.Now(),
	}
	h.hub.SaveUser(user)

	token, err := middleware.GenerateToken(user)
	if err != nil {
		writeError(w, "token error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, models.AuthResponse{Token: token, User: *user}, http.StatusCreated)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, exists := h.hub.GetUserByEmail(req.Email)
	if !exists {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := middleware.GenerateToken(user)
	if err != nil {
		writeError(w, "token error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, models.AuthResponse{Token: token, User: *user}, http.StatusOK)
}

func (h *Handler) GetRooms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.hub.GetRooms(), http.StatusOK)
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUser(r)
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, "name required", http.StatusBadRequest)
		return
	}

	room := &models.Room{
		ID:          uuid.New().String(),
		Name:        "# " + body.Name,
		Description: body.Description,
		CreatedBy:   claims.UserID,
		CreatedAt:   time.Now(),
		Private:     body.Private,
	}
	h.hub.CreateRoom(room)
	writeJSON(w, room, http.StatusCreated)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomID"]
	if _, ok := h.hub.GetRoom(roomID); !ok {
		writeError(w, "room not found", http.StatusNotFound)
		return
	}
	writeJSON(w, h.hub.GetMessages(roomID), http.StatusOK)
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUser(r)
	roomID := mux.Vars(r)["roomID"]

	if _, ok := h.hub.GetRoom(roomID); !ok {
		writeError(w, "room not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	h.hub.ServeWS(conn, claims.UserID, claims.Username, claims.Avatar, roomID)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func writeJSON(w http.ResponseWriter, v interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, map[string]string{"error": msg}, status)
}
