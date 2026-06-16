package email

import "github.com/elad/rolebook-backend/config"

// defaultFrom is Resend's shared onboarding sender, usable before a custom
// domain is verified. Override with EMAIL_FROM in production.
const defaultFrom = "Rolebook <onboarding@resend.dev>"

// New returns a ResendSender when RESEND_API_KEY is configured, otherwise a
// LogSender so dev and CI need no external provider. Mirrors avatarstore.New.
func New(cfg config.Config) Sender {
	if cfg.ResendAPIKey == "" {
		return LogSender{}
	}
	from := cfg.EmailFrom
	if from == "" {
		from = defaultFrom
	}
	return NewResendSender(cfg.ResendAPIKey, from)
}
