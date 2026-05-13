package handler

import (
	"errors"
	"strings"
)

// sessionNoteMaxLength caps the size of a single session note (in characters).
const sessionNoteMaxLength = 10_000

// errSessionNoteTooLong signals that the input exceeds sessionNoteMaxLength.
var errSessionNoteTooLong = errors.New("session note text too long")

// normalizeSessionNoteText trims leading/trailing whitespace and decides
// whether the input represents a clear (empty/whitespace) or an upsert.
// It rejects text longer than sessionNoteMaxLength.
//
// Returns:
//   - text:    the trimmed text (empty when cleared)
//   - cleared: true when the trimmed input is empty
//   - err:     non-nil when the input is too long
func normalizeSessionNoteText(s string) (text string, cleared bool, err error) {
	if len(s) > sessionNoteMaxLength {
		return "", false, errSessionNoteTooLong
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", true, nil
	}
	return trimmed, false, nil
}
