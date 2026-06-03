package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type Manager struct {
	mu       sync.RWMutex
	tokens   map[string]string
	reserved map[string]struct{}
}

func NewManager(staticTokens map[string]string) *Manager {
	m := &Manager{
		tokens:   make(map[string]string),
		reserved: make(map[string]struct{}),
	}
	for token, user := range staticTokens {
		m.tokens[token] = user
		m.reserved[token] = struct{}{}
	}
	return m
}

func (m *Manager) IssueToken(user string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[token] = user
	return token
}

func (m *Manager) Validate(token string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	user, ok := m.tokens[token]
	return user, ok
}

func (m *Manager) Revoke(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, isReserved := m.reserved[token]; isReserved {
		return false
	}
	if _, ok := m.tokens[token]; !ok {
		return false
	}
	delete(m.tokens, token)
	return true
}
