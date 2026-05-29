// Package email delivers transactional email behind a provider-agnostic
// Sender interface, so the concrete provider (currently Resend) can be swapped
// by changing only this package.
package email

import "context"

// Sender delivers a single transactional email. Implementations must be safe
// for concurrent use.
type Sender interface {
	Send(ctx context.Context, to, subject, htmlBody, textBody string) error
}
