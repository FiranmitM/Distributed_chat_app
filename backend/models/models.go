package models

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Avatar       string    `json:"avatar"`
	CreatedAt    time.Time `json:"created_at"`
	Online       bool      `json:"online"`
}

type Room struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	MemberCount int       `json:"member_count"`
	Private     bool      `json:"private"`
}

type Message struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // text, image, system
	CreatedAt time.Time `json:"created_at"`
}

type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	RoomID  string      `json:"room_id,omitempty"`
}

type AuthRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}
