package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/FiranmitM/Distributed_chat_app/models"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

var jwtSecret []byte

func SetSecret(secret string) {
	jwtSecret = []byte(secret)
}

func GenerateToken(user *models.User) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"avatar":   user.Avatar,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})
	return token.SignedString(jwtSecret)
}

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		tokenStr := ""

		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		claims, err := parseToken(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseToken(tokenStr string) (*models.Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}

	mc := token.Claims.(jwt.MapClaims)
	return &models.Claims{
		UserID:   mc["user_id"].(string),
		Username: mc["username"].(string),
		Avatar:   mc["avatar"].(string),
	}, nil
}

func GetUser(r *http.Request) *models.Claims {
	if v := r.Context().Value(UserContextKey); v != nil {
		return v.(*models.Claims)
	}
	return nil
}
