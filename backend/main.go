package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FiranmitM/Distributed_chat_app/config"
	"github.com/FiranmitM/Distributed_chat_app/handlers"
	"github.com/FiranmitM/Distributed_chat_app/hub"
	"github.com/FiranmitM/Distributed_chat_app/middleware"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
)

func main() {
	cfg := config.Load()
	middleware.SetSecret(cfg.JWTSecret)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️  Redis unavailable (%v) — running in single-node mode", err)
	} else {
		log.Printf("✅ Connected to Redis at %s", cfg.RedisAddr)
	}
	cancel()

	h := hub.NewHub(rdb, cfg.NodeID)
	go h.Run(context.Background())

	handler := handlers.New(h)
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/health", handler.Health).Methods("GET")
	r.HandleFunc("/api/auth/register", handler.Register).Methods("POST")
	r.HandleFunc("/api/auth/login", handler.Login).Methods("POST")

	// Protected routes
	api := r.PathPrefix("/api").Subrouter()
	api.Use(middleware.Auth)
	api.HandleFunc("/rooms", handler.GetRooms).Methods("GET")
	api.HandleFunc("/rooms", handler.CreateRoom).Methods("POST")
	api.HandleFunc("/rooms/{roomID}/messages", handler.GetMessages).Methods("GET")

	// WebSocket
	r.Handle("/ws/{roomID}", middleware.Auth(http.HandlerFunc(handler.ServeWS)))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      c.Handler(r),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 [%s] Server listening on :%s", cfg.NodeID, cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}
