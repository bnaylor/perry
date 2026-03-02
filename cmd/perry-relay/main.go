package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/config"
	"github.com/bnaylor/perry/internal/discord"
	"github.com/bnaylor/perry/internal/storage"
	"github.com/bnaylor/perry/internal/task"
	"github.com/bwmarrin/discordgo"
)

const (
	cursorTransitionsKey = "discord_relay_cursor_transitions"
	cursorCallsKey       = "discord_relay_cursor_calls"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dbPath := flag.String("db-path", ".perry/perry.db", "path to SQLite database")
	discordCfgPath := flag.String("discord-config", "configs/discord.yaml", "path to discord config")
	flag.Parse()

	// Load Discord Config
	dCfg, err := config.LoadDiscordConfig(*discordCfgPath)
	if err != nil {
		slog.Error("failed to load discord config", "error", err)
		os.Exit(1)
	}

	// Map roles from config to agent.Role
	activeTokens := make(map[agent.Role]string)
	roleMap := map[string]agent.Role{
		"strategist":     agent.RoleStrategist,
		"researcher":     agent.RoleResearcher,
		"coder":          agent.RoleCoder,
		"auditor":        agent.RoleAuditor,
		"shadow_auditor": agent.RoleShadowAuditor,
		"orchestrator":   agent.Role("orchestrator"), // Agent P
	}

	for cfgKey, agentRole := range roleMap {
		if role, ok := dCfg.Roles[cfgKey]; ok && role.Token != "" {
			activeTokens[agentRole] = role.Token
		}
	}

	if len(activeTokens) == 0 {
		slog.Error("no discord tokens found in config")
		os.Exit(1)
	}

	// Initialize Storage
	store, err := storage.NewStore(*dbPath)
	if err != nil {
		slog.Error("failed to open storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize Discord Client
	client := discord.NewClient(activeTokens)
	if err := client.Start(); err != nil {
		slog.Error("failed to start discord client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle inbound commands
	client.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		if len(m.Content) > 0 && m.Content[0] == '!' {
			slog.Info("received command from discord", "content", m.Content, "author", m.Author.Username)
			
			taskID := "" // TODO: Extract taskID if in thread
			_, err = store.AddCommand(ctx, taskID, m.Content, m.ChannelID)
			if err != nil {
				slog.Error("failed to write command to db", "error", err)
			}
		}
	})

	go runPoller(ctx, store, client, dCfg.Channels.Coordination)

	slog.Info("perry-relay started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down perry-relay")
}

func runPoller(ctx context.Context, store *storage.Store, client *discord.Client, mainChannelID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pollTransitions(ctx, store, client, mainChannelID); err != nil {
				slog.Error("transitions poller error", "error", err)
			}
			if err := pollAgentCalls(ctx, store, client, mainChannelID); err != nil {
				slog.Error("agent calls poller error", "error", err)
			}
		}
	}
}

func pollTransitions(ctx context.Context, store *storage.Store, client *discord.Client, mainChannelID string) error {
	cursorStr, err := store.GetInternalState(ctx, cursorTransitionsKey)
	if err != nil {
		return err
	}
	cursor, _ := strconv.ParseInt(cursorStr, 10, 64)

	records, err := store.GetTransitionsSince(ctx, cursor)
	if err != nil {
		return err
	}

	for _, r := range records {
		tk, err := store.Get(ctx, r.TaskID)
		if err != nil {
			slog.Warn("failed to fetch task for transition", "task", r.TaskID, "error", err)
			continue
		}

		targetChannel := tk.DiscordThreadID
		if targetChannel == "" {
			targetChannel = mainChannelID
		}

		// Handle thread creation on SUBMITTED
		if task.State(r.ToState) == task.StateSubmitted && tk.DiscordThreadID == "" {
			threadName := fmt.Sprintf("%s: %s", tk.ID, tk.Description)
			if len(threadName) > 100 {
				threadName = threadName[:97] + "..."
			}
			threadID, err := client.CreateThread(agent.RoleStrategist, mainChannelID, threadName)
			if err != nil {
				slog.Error("failed to create thread", "task", tk.ID, "error", err)
			} else {
				tk.DiscordThreadID = threadID
				if err := store.Update(ctx, tk); err != nil {
					slog.Error("failed to update task with thread id", "task", tk.ID, "error", err)
				}
				targetChannel = threadID
			}
		}

		// Format and send message
		templateName := discord.MapStateToTemplate(task.State(r.FromState), task.State(r.ToState))
		content, err := discord.Format(templateName, discord.EventData{
			TaskID:      tk.ID,
			Description: tk.Description,
			FromState:   r.FromState,
			ToState:     r.ToState,
			Reason:      r.Reason,
		})
		if err == nil {
			// Always send transitions as Agent P (orchestrator role in config)
			if err := client.SendMessage(agent.Role("orchestrator"), targetChannel, content); err != nil {
				slog.Error("failed to send transition message", "error", err)
			}
		}

		cursor = r.ID
	}

	if len(records) > 0 {
		return store.SetInternalState(ctx, cursorTransitionsKey, strconv.FormatInt(cursor, 10))
	}
	return nil
}

func pollAgentCalls(ctx context.Context, store *storage.Store, client *discord.Client, mainChannelID string) error {
	cursorStr, err := store.GetInternalState(ctx, cursorCallsKey)
	if err != nil {
		return err
	}
	cursor, _ := strconv.ParseInt(cursorStr, 10, 64)

	records, err := store.GetAgentCallsSince(ctx, cursor)
	if err != nil {
		return err
	}

	for _, r := range records {
		tk, err := store.Get(ctx, r.TaskID)
		if err != nil {
			continue
		}

		targetChannel := tk.DiscordThreadID
		if targetChannel == "" {
			targetChannel = mainChannelID
		}

		// Map role to agent name for template
		agentName := r.Role // Fallback
		switch agent.Role(r.Role) {
		case agent.RoleStrategist:
			agentName = "Major Monogram"
		case agent.RoleResearcher:
			agentName = "Phineas"
		case agent.RoleCoder:
			agentName = "Ferb"
		case agent.RoleAuditor:
			agentName = "Carl"
		case agent.RoleShadowAuditor:
			agentName = "Dr. Doofenshmirtz"
		}

		content, err := discord.Format("agent_call", discord.EventData{
			AgentName: agentName,
			Role:      r.Role,
			Content:   r.Content,
		})
		if err == nil {
			if err := client.SendMessage(agent.Role(r.Role), targetChannel, content); err != nil {
				slog.Error("failed to send agent call message", "error", err)
			}
		}

		cursor = r.ID
	}

	if len(records) > 0 {
		return store.SetInternalState(ctx, cursorCallsKey, strconv.FormatInt(cursor, 10))
	}
	return nil
}
