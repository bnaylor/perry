package discord

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bwmarrin/discordgo"
)

// Persona represents a themed bot identity.
type Persona struct {
	Name  string
	Vibe  string
	Token string
}

// Map roles to Phineas & Ferb personas.
var defaultPersonas = map[agent.Role]string{
	agent.RoleStrategist:    "Major Monogram",
	agent.RoleResearcher:    "Phineas",
	agent.RoleCoder:         "Ferb",
	agent.RoleAuditor:       "Carl",
	agent.RoleShadowAuditor: "Doof",
}

// Client manages multiple Discord bot sessions for the roundtable cast.
type Client struct {
	sessions map[agent.Role]*discordgo.Session
	tokens   map[agent.Role]string
	mu       sync.RWMutex
}

// NewClient creates a new multi-bot Discord client.
func NewClient(tokens map[agent.Role]string) *Client {
	return &Client{
		sessions: make(map[agent.Role]*discordgo.Session),
		tokens:   tokens,
	}
}

// Start initializes connections for all configured bots.
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
		slog.Info("discord session opened", "role", role, "persona", defaultPersonas[role])
	}

	return nil
}

// Close gracefully shuts down all active sessions.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for role, dg := range c.sessions {
		slog.Info("closing discord session", "role", role)
		dg.Close()
	}
}

// CreateThread creates a new thread in the given channel.
func (c *Client) CreateThread(role agent.Role, channelID, name string) (string, error) {
	session, ok := c.getSession(role)
	if !ok {
		return "", fmt.Errorf("no active session for role: %s", role)
	}

	// Create thread (StartThread was added in newer discordgo versions)
	thread, err := session.ThreadStart(channelID, name, discordgo.ChannelTypeGuildPublicThread, 60)
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	return thread.ID, nil
}

// SendMessage posts a message to a channel or thread as the given agent role.
func (c *Client) SendMessage(role agent.Role, channelID, content string) error {
	session, ok := c.getSession(role)
	if !ok {
		return fmt.Errorf("no active session for role: %s", role)
	}

	_, err := session.ChannelMessageSend(channelID, content)
	return err
}

func (c *Client) getSession(role agent.Role) (*discordgo.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Fallback to Strategist if specific role session isn't available
	s, ok := c.sessions[role]
	if !ok {
		s, ok = c.sessions[agent.RoleStrategist]
	}
	return s, ok
}

// AddHandler registers a message handler for all active sessions.
func (c *Client) AddHandler(handler any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, dg := range c.sessions {
		dg.AddHandler(handler)
	}
}
