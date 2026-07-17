package resetstore

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	session   Session
	expiresAt time.Time
}

// Memory is an in-process Store for local dev and tests. Entries expire lazily
// on read. A parallel cooldown map tracks resend windows.
type Memory struct {
	mu        sync.Mutex
	sessions  map[string]memEntry
	cooldowns map[string]time.Time // email -> cooldown-until
	now       func() time.Time     // overridable in tests
}

func NewMemory() *Memory {
	return &Memory{
		sessions:  make(map[string]memEntry),
		cooldowns: make(map[string]time.Time),
		now:       time.Now,
	}
}

func (m *Memory) MarkSent(_ context.Context, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if until, ok := m.cooldowns[email]; ok && now.Before(until) {
		return false, nil
	}
	m.cooldowns[email] = now.Add(CooldownTTL)
	return true, nil
}

func (m *Memory) SetCode(_ context.Context, email, codeHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[email] = memEntry{
		session:   Session{CodeHash: codeHash},
		expiresAt: m.now().Add(CodeTTL),
	}
	return nil
}

func (m *Memory) Get(_ context.Context, email string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[email]
	if !ok || m.now().After(e.expiresAt) {
		if ok {
			delete(m.sessions, email)
		}
		return nil, nil
	}
	s := e.session
	return &s, nil
}

func (m *Memory) IncrAttempts(_ context.Context, email string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[email]
	if !ok || m.now().After(e.expiresAt) {
		return 0, nil
	}
	e.session.Attempts++
	m.sessions[email] = e
	return e.session.Attempts, nil
}

func (m *Memory) PromoteToToken(_ context.Context, email, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[email] = memEntry{
		session:   Session{TokenHash: tokenHash},
		expiresAt: m.now().Add(CodeTTL),
	}
	return nil
}

func (m *Memory) Clear(_ context.Context, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, email)
	delete(m.cooldowns, email)
	return nil
}
