package config

import "os"

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port      string
	MongoURI  string
	JWTSecret string
}

// Load reads configuration from environment variables.
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	return Config{
		Port:      port,
		MongoURI:  mustEnv("MONGO_URI"),
		JWTSecret: mustEnv("JWT_SECRET"),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}
