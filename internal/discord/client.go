package discord

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bwmarrin/discordgo"
)

// Sender defines the interface for sending messages and managing threads.
type Sender interface {
	Start() error
	Close()
	CreateThread(role agent.Role, channelID, name string) (string, error)
	SendMessage(role agent.Role, channelID, content string) error
	RegisterHandler(role agent.Role, handler any) error
}

// DefaultPersonas maps roles to Phineas & Ferb personas.
var DefaultPersonas = map[agent.Role]string{
	agent.RoleStrategist:    "Major Monogram",
	agent.RoleResearcher:    "Phineas",
	agent.RoleCoder:         "Ferb",
	agent.RoleAuditor:       "Carl",
	agent.RoleShadowAuditor: "Doof",
}

// Client manages multiple Discord bot sessions.
type Client struct {
	sessions map[agent.Role]*discordgo.Session
	tokens   map[agent.Role]string
	mu       sync.RWMutex
}

func NewClient(tokens map[agent.Role]string) *Client {
	return &Client{
		sessions: make(map[agent.Role]*discordgo.Session),
		tokens:   tokens,
	}
}

func (c *Client) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for role, token := range c.tokens {
		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			return fmt.Errorf("failed to create session for %s: %w", role, err)
		}
		if err := dg.Open(); err != nil {
			return fmt.Errorf("failed to open session for %s: %w", role, err)
		}
		c.sessions[role] = dg
	}
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, dg := range c.sessions {
		dg.Close()
	}
}

func (c *Client) CreateThread(role agent.Role, channelID, name string) (string, error) {
	session, ok := c.getSession(role)
	if !ok {
		return "", fmt.Errorf("no active session for role: %s", role)
	}
	thread, err := session.ThreadStart(channelID, name, discordgo.ChannelTypeGuildPublicThread, 60)
	if err != nil {
		return "", err
	}
	return thread.ID, nil
}

func (c *Client) SendMessage(role agent.Role, channelID, content string) error {
	session, ok := c.getSession(role)
	if !ok {
		return fmt.Errorf("no active session for role: %s", role)
	}
	_, err := session.ChannelMessageSend(channelID, content)
	return err
}

func (c *Client) RegisterHandler(role agent.Role, handler any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, ok := c.sessions[role]
	if !ok {
		return fmt.Errorf("no active session for role: %s", role)
	}
	session.AddHandler(handler)
	return nil
}

func (c *Client) getSession(role agent.Role) (*discordgo.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.sessions[role]
	if !ok {
		s, ok = c.sessions[agent.RoleStrategist]
	}
	return s, ok
}

// MockClient logs messages to a file instead of sending to Discord.
type MockClient struct {
	LogPath string
	file    *os.File
}

func (m *MockClient) Start() error {
	f, err := os.OpenFile(m.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	m.file = f
	fmt.Fprintf(m.file, "\n--- Mock Session Started at %s ---\n", time.Now().Format(time.RFC3339))
	return nil
}

func (m *MockClient) Close() {
	if m.file != nil {
		m.file.Close()
	}
}

func (m *MockClient) CreateThread(role agent.Role, channelID, name string) (string, error) {
	id := fmt.Sprintf("mock-thread-%d", time.Now().UnixNano())
	if m.file != nil {
		fmt.Fprintf(m.file, "[%s] CREATE THREAD in %s: %s (ID: %s)\n", role, channelID, name, id)
	}
	return id, nil
}

func (m *MockClient) SendMessage(role agent.Role, channelID, content string) error {
	if m.file != nil {
		fmt.Fprintf(m.file, "[%s] SEND TO %s: %s\n", role, channelID, content)
	}
	return nil
}

func (m *MockClient) RegisterHandler(role agent.Role, handler any) error {
	slog.Info("mock registering handler", "role", role)
	return nil
}
