package tenants

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type CaptureMailer struct {
	mu       sync.RWMutex
	baseURL  string
	messages []CaptureMessage
}

func NewCaptureMailer(baseURL string) *CaptureMailer {
	return &CaptureMailer{baseURL: strings.TrimRight(baseURL, "/")}
}

func (m *CaptureMailer) SendInvitation(_ context.Context, email, token, tenantName string, expiresAt time.Time) error {
	if m.baseURL == "" {
		return fmt.Errorf("local invitation URL is not configured")
	}
	invitationURL := m.baseURL + "/invite/" + url.PathEscape(token)
	message := CaptureMessage{
		To: email, Subject: "Activate your School Without Walls school owner account",
		Body:      fmt.Sprintf("You were invited to set up %s as its school owner.\n\nAccept invitation: %s\n\nThis one-time link expires at %s.\n", tenantName, invitationURL, expiresAt.UTC().Format(time.RFC3339)),
		CreatedAt: time.Now().UTC(),
	}
	m.mu.Lock()
	m.messages = append(m.messages, message)
	m.mu.Unlock()
	return nil
}

func (m *CaptureMailer) Messages() []CaptureMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	messages := make([]CaptureMessage, len(m.messages))
	copy(messages, m.messages)
	return messages
}
