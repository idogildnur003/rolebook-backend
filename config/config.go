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
	AWSS3UsePathStyle  bool

	AdminUserIDs []string
}

// Load reads configuration from environment variables.
// portFlag, when non-empty, overrides the PORT env var (e.g. from a -port CLI flag).
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load(portFlag string) Config {
	resendKey := os.Getenv("RESEND_API_KEY")
	return Config{
		Port:                     resolvePort(portFlag, os.Getenv("PORT")),
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
		AWSS3UsePathStyle:        envBool(os.Getenv("AWS_S3_FORCE_PATH_STYLE")),
		AdminUserIDs:             parseCSV(os.Getenv("ADMIN_USER_IDS")),
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
