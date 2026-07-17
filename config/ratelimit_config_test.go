package config

import (
	"testing"
	"time"
)

func TestEnvInt(t *testing.T) {
	if got := envInt("RL_TEST_INT_UNSET", 42); got != 42 {
		t.Errorf("unset: got %d, want 42", got)
	}
	t.Setenv("RL_TEST_INT", "123")
	if got := envInt("RL_TEST_INT", 42); got != 123 {
		t.Errorf("set: got %d, want 123", got)
	}
	t.Setenv("RL_TEST_INT_BAD", "notanumber")
	if got := envInt("RL_TEST_INT_BAD", 42); got != 42 {
		t.Errorf("unparseable: got %d, want 42 (default)", got)
	}
	t.Setenv("RL_TEST_INT_BLANK", "   ")
	if got := envInt("RL_TEST_INT_BLANK", 7); got != 7 {
		t.Errorf("blank: got %d, want 7 (default)", got)
	}
}

func TestEnvDuration(t *testing.T) {
	if got := envDuration("RL_TEST_DUR_UNSET", time.Minute); got != time.Minute {
		t.Errorf("unset: got %v, want 1m", got)
	}
	t.Setenv("RL_TEST_DUR", "30s")
	if got := envDuration("RL_TEST_DUR", time.Minute); got != 30*time.Second {
		t.Errorf("set: got %v, want 30s", got)
	}
	t.Setenv("RL_TEST_DUR_BAD", "abc")
	if got := envDuration("RL_TEST_DUR_BAD", time.Minute); got != time.Minute {
		t.Errorf("unparseable: got %v, want 1m (default)", got)
	}
}

func TestEnvBoolDefault(t *testing.T) {
	if !envBoolDefault("RL_TEST_BOOL_UNSET", true) {
		t.Error("unset with default true should be true")
	}
	if envBoolDefault("RL_TEST_BOOL_UNSET2", false) {
		t.Error("unset with default false should be false")
	}
	t.Setenv("RL_TEST_BOOL_OFF", "false")
	if envBoolDefault("RL_TEST_BOOL_OFF", true) {
		t.Error("explicit false should override default true")
	}
	t.Setenv("RL_TEST_BOOL_ON", "1")
	if !envBoolDefault("RL_TEST_BOOL_ON", false) {
		t.Error("explicit 1 should override default false")
	}
}
