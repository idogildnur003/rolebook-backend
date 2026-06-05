package config

import (
	"os"
	"strings"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port      string
	MongoURI  string
	JWTSecret string

	AWSS3Endpoint      string
	AWSRegion          string
	AWSS3Bucket        string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSS3UsePathStyle  bool

	AdminUserIDs []string
}

// Load reads configuration from environment variables.
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	return Config{
		Port:               port,
		MongoURI:           mustEnv("MONGO_URI"),
		JWTSecret:          mustEnv("JWT_SECRET"),
		AWSS3Endpoint:      os.Getenv("AWS_ENDPOINT_URL"),
		AWSRegion:          os.Getenv("AWS_REGION"),
		AWSS3Bucket:        os.Getenv("AWS_S3_BUCKET"),
		AWSAccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSS3UsePathStyle:  envBool(os.Getenv("AWS_S3_FORCE_PATH_STYLE")),
		AdminUserIDs:       parseCSV(os.Getenv("ADMIN_USER_IDS")),
	}
}

// parseCSV splits a comma-separated env value into trimmed, non-empty parts.
// Returns nil for an empty/blank input.
func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// envBool parses a boolean-ish env value. True for "true", "1", "yes", "on"
// (case-insensitive); false otherwise (including empty).
func envBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}
