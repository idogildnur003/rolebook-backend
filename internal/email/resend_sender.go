package email

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/goccy/go-json"
)

const resendEndpoint = "https://api.resend.com/emails"

// ResendSender sends email through the Resend HTTP API
// (https://resend.com/docs/api-reference/emails/send-email).
type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
}

// NewResendSender builds a ResendSender. from is the verified sender, e.g.
// "Rolebook <noreply@yourdomain.com>".
func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts one email to the Resend API.
func (s *ResendSender) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	payload := map[string]any{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"html":    htmlBody,
		"text":    textBody,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// Surface Resend's response body and the from address — these carry the
		// actual reason (unverified domain, invalid from, testing-mode recipient
		// restriction, etc.) that a bare status code hides.
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("resend: status %d (from %q): %s", resp.StatusCode, s.from, bytes.TrimSpace(respBody))
	}
	return nil
}
