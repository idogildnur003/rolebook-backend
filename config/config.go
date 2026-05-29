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

	ResendAPIKey string
	EmailFrom    string
	// EmailVerificationEnabled gates the entire OTP flow. Default = whether a
	// Resend key is configured, so local dev with no key skips verification and
	// production with a key enforces it; explicit EMAIL_VERIFICATION_ENABLED
	// overrides for either side (e.g. CI testing the flow, or an incident
	// switch in prod).
	EmailVerificationEnabled bool

	AWSS3Endpoint      string
	AWSRegion          string
	AWSS3Bucket        string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
}

// Load reads configuration from environment variables.
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	resendKey := os.Getenv("RESEND_API_KEY")
	return Config{
		Port:                     port,
		MongoURI:                 mustEnv("MONGO_URI"),
		JWTSecret:                mustEnv("JWT_SECRET"),
		ResendAPIKey:             resendKey,
		EmailFrom:                os.Getenv("EMAIL_FROM"),
		EmailVerificationEnabled: emailVerificationEnabled(resendKey),
		AWSS3Endpoint:            os.Getenv("AWS_ENDPOINT_URL"),
		AWSRegion:                os.Getenv("AWS_REGION"),
		AWSS3Bucket:              os.Getenv("AWS_S3_BUCKET"),
		AWSAccessKeyID:           os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:       os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}

// emailVerificationEnabled resolves the flag: explicit EMAIL_VERIFICATION_ENABLED
// (true/false/1/0) wins; otherwise defaults to whether a Resend key is set.
func emailVerificationEnabled(resendKey string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_VERIFICATION_ENABLED"))) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return resendKey != ""
}
