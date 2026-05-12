package config

import "os"

type Config struct {
	Port      string
	RedisAddr string
	JWTSecret string
	NodeID    string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		JWTSecret: getEnv("JWT_SECRET", "super-secret-distributed-chat-key-2024"),
		NodeID:    getEnv("NODE_ID", "node-1"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
