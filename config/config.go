package config

import "os"

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
}

// Load reads configuration from environment variables.
// portFlag, when non-empty, overrides the PORT env var (e.g. from a -port CLI flag).
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load(portFlag string) Config {
	return Config{
		Port:               resolvePort(portFlag, os.Getenv("PORT")),
		MongoURI:           mustEnv("MONGO_URI"),
		JWTSecret:          mustEnv("JWT_SECRET"),
		AWSS3Endpoint:      os.Getenv("AWS_ENDPOINT_URL"),
		AWSRegion:          os.Getenv("AWS_REGION"),
		AWSS3Bucket:        os.Getenv("AWS_S3_BUCKET"),
		AWSAccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}
}

// resolvePort selects the HTTP listen port with precedence: flag > PORT env > "3000".
func resolvePort(flagVal, envVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	return "3000"
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}
