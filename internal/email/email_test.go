package email

import (
	"context"
	"testing"

	"github.com/elad/rolebook-backend/config"
)

func TestNew_NoAPIKeyReturnsLogSender(t *testing.T) {
	s := New(config.Config{})
	if _, ok := s.(LogSender); !ok {
		t.Fatalf("New with no API key = %T, want LogSender", s)
	}
}

func TestNew_WithAPIKeyReturnsResendSender(t *testing.T) {
	s := New(config.Config{ResendAPIKey: "re_test"})
	if _, ok := s.(*ResendSender); !ok {
		t.Fatalf("New with API key = %T, want *ResendSender", s)
	}
}

func TestNew_DefaultsFromWhenEmpty(t *testing.T) {
	s := New(config.Config{ResendAPIKey: "re_test"}).(*ResendSender)
	if s.from != defaultFrom {
		t.Errorf("from = %q, want default %q", s.from, defaultFrom)
	}
}

func TestLogSender_SendNeverErrors(t *testing.T) {
	if err := (LogSender{}).Send(context.Background(), "a@b.com", "subj", "<p>x</p>", "x"); err != nil {
		t.Fatalf("LogSender.Send returned error: %v", err)
	}
}
