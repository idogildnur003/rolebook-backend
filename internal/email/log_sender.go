package email

import (
	"context"
	"log"
)

// LogSender writes emails to the application log instead of sending them. It is
// selected automatically when no provider API key is configured, so local dev
// and CI can read verification codes straight from the server log.
type LogSender struct{}

// Send logs the recipient, subject, and plain-text body. Never errors.
func (LogSender) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	log.Printf("[email:log] to=%s subject=%q\n%s", to, subject, textBody)
	return nil
}
