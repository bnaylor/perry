package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	testMode := flag.Bool("test-mode", false, "log messages to file instead of sending to discord")
	channelID := flag.String("channel-id", "1477155447260577917", "coordination channel ID override")
	flag.Parse()

	dCfg, err := config.LoadDiscordConfig(*discordCfgPath)
	if err != nil {
		slog.Error("failed to load discord config", "error", err)
		os.Exit(1)
	}

	finalChannelID := dCfg.Channels.Coordination
	if *channelID != "" && *channelID != "1477155447260577917" {
		finalChannelID = *channelID
	}

	activeTokens := make(map[agent.Role]string)
	roleMap := map[string]agent.Role{
		"strategist":     agent.RoleStrategist,
		"researcher":     agent.RoleResearcher,
		"coder":          agent.RoleCoder,
		"auditor":        agent.RoleAuditor,
		"shadow_auditor": agent.RoleShadowAuditor,
		"orchestrator":   agent.Role("orchestrator"),
	}

	for cfgKey, agentRole := range roleMap {
		if role, ok := dCfg.Roles[cfgKey]; ok && role.Token != "" {
			activeTokens[agentRole] = role.Token
		}
	}

	store, err := storage.NewStore(*dbPath)
	if err != nil {
		slog.Error("failed to open storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	var client discord.Sender
	if *testMode {
		slog.Info("running in TEST MODE - messages will be logged to perry_relay_mock.log")
		client = &discord.MockClient{LogPath: "perry_relay_mock.log"}
	} else {
		if len(activeTokens) == 0 {
			slog.Error("no discord tokens found in config")
			os.Exit(1)
		}
		client = discord.NewClient(activeTokens)
	}

	if err := client.Start(); err != nil {
		slog.Error("failed to start discord client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client.RegisterHandler(agent.RoleStrategist, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		if len(m.Content) > 0 && m.Content[0] == '!' {
			slog.Info("received command from discord", "content", m.Content, "author", m.Author.Username)
			taskID := "" 
			_, err = store.AddCommand(ctx, taskID, m.Content, m.ChannelID)
			if err != nil {
				slog.Error("failed to write command to db", "error", err)
			}
		}
	})

	go runPoller(ctx, store, client, finalChannelID)

	slog.Info("perry-relay started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down perry-relay")
}

func runPoller(ctx context.Context, store *storage.Store, client discord.Sender, mainChannelID string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollTransitions(ctx, store, client, mainChannelID)
			pollAgentCalls(ctx, store, client, mainChannelID)
		}
	}
}

func pollTransitions(ctx context.Context, store *storage.Store, client discord.Sender, mainChannelID string) {
	cursorStr, err := store.GetInternalState(ctx, cursorTransitionsKey)
	if err != nil {
		slog.Error("failed to get transitions cursor", "error", err)
		return
	}
	cursor, _ := strconv.ParseInt(cursorStr, 10, 64)

	records, err := store.GetTransitionsSince(ctx, cursor)
	if err != nil {
		slog.Error("failed to get transitions", "error", err)
		return
	}

	for _, r := range records {
		slog.Info("processing transition", "id", r.ID, "task", r.TaskID, "from", r.FromState, "to", r.ToState)
		tk, err := store.Get(ctx, r.TaskID)
		if err != nil {
			slog.Warn("failed to fetch task for transition", "task", r.TaskID, "error", err)
			store.SetInternalState(ctx, cursorTransitionsKey, strconv.FormatInt(r.ID, 10))
			continue
		}

		if tk.DiscordThreadID == "" {
			if time.Since(tk.CreatedAt) < 24*time.Hour {
				threadName := fmt.Sprintf("%s: %s", tk.ID, tk.Description)
				if len(threadName) > 100 {
					threadName = threadName[:97] + "..."
				}
				threadID, err := client.CreateThread(agent.RoleStrategist, mainChannelID, threadName)
				if err != nil {
					slog.Error("failed to create thread", "task", tk.ID, "error", err)
				} else {
					tk.DiscordThreadID = threadID
					if err := store.SetTaskThreadID(ctx, tk.ID, threadID); err != nil {
						slog.Error("failed to update task with thread id", "task", tk.ID, "error", err)
					}
				}
			}
		}

		targetChannel := tk.DiscordThreadID
		if targetChannel == "" {
			targetChannel = mainChannelID
		}

		templateName := discord.MapStateToTemplate(task.State(r.FromState), task.State(r.ToState))
		if task.State(r.FromState) == task.StateSubmitted {
			templateName = "task_submitted"
		}

		content, err := discord.Format(templateName, discord.EventData{
			TaskID:      tk.ID,
			Description: tk.Description,
			FromState:   r.FromState,
			ToState:     r.ToState,
			Reason:      r.Reason,
		})
		if err == nil {
			if len(content) > 2000 {
				content = content[:1997] + "..."
			}
			if err := client.SendMessage(agent.Role("orchestrator"), targetChannel, content); err != nil {
				slog.Error("failed to send transition message", "error", err)
			}
		}

		if err := store.SetInternalState(ctx, cursorTransitionsKey, strconv.FormatInt(r.ID, 10)); err != nil {
			slog.Error("failed to update transitions cursor", "id", r.ID, "error", err)
			return
		}
	}
}

func pollAgentCalls(ctx context.Context, store *storage.Store, client discord.Sender, mainChannelID string) {
	cursorStr, err := store.GetInternalState(ctx, cursorCallsKey)
	if err != nil {
		slog.Error("failed to get agent calls cursor", "error", err)
	case agent.RoleShadowAuditor:
		if found, ok := data["vulnerability_found"].(bool); ok {
			backstory := ""
			if bs, ok := data["backstory"].(string); ok {
				backstory = "\n\n**Backstory:** " + bs
			}

			if found {
				return "🚨 **Vulnerability identified!**" + backstory
			}
			return "No vulnerabilities found in this sweep." + backstory
		}
		slog.Info("processing agent call", "id", r.ID, "task", r.TaskID, "role", r.Role)
		tk, err := store.Get(ctx, r.TaskID)
		if err != nil {
			store.SetInternalState(ctx, cursorCallsKey, strconv.FormatInt(r.ID, 10))
			continue
		}

		targetChannel := tk.DiscordThreadID
		if targetChannel == "" {
			targetChannel = mainChannelID
		}

		// Map role to persona from internal/discord
		persona := discord.DefaultPersonas[agent.Role(r.Role)]
		if persona == "" {
			persona = r.Role
		}

		displayContent := r.Content
		// Remove markdown blocks if present
		displayContent = strings.TrimPrefix(displayContent, "```json")
		displayContent = strings.TrimPrefix(displayContent, "```")
		displayContent = strings.TrimSuffix(displayContent, "```")
		displayContent = strings.TrimSpace(displayContent)

		if strings.HasPrefix(displayContent, "{") {
			displayContent = summarizeJSON(r.Role, displayContent)
		}

		content, err := discord.Format("agent_call", discord.EventData{
			Persona:   persona,
			Role:      r.Role,
			Content:   displayContent,
		})
		if err == nil {
			if len(content) > 2000 {
				content = content[:1997] + "..."
			}
			if err := client.SendMessage(agent.Role(r.Role), targetChannel, content); err != nil {
				slog.Error("failed to send agent call message", "error", err)
			}
		}

		if err := store.SetInternalState(ctx, cursorCallsKey, strconv.FormatInt(r.ID, 10)); err != nil {
			slog.Error("failed to update agent calls cursor", "id", r.ID, "error", err)
			return
		}
	}
}

func summarizeJSON(role, raw string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw 
	}

	switch agent.Role(role) {
	case agent.RoleStrategist:
		if summary, ok := data["task_summary"].(string); ok {
			return summary
		}
		if prd, ok := data["prd_summary"].(string); ok {
			return prd
		}
	case agent.RoleCoder:
		if err, ok := data["error"].(string); ok {
			return "Error: " + err
		}
		if code, ok := data["code"].(string); ok {
			lines := strings.Split(code, "\n")
			if len(lines) > 5 {
				return "Generated " + strconv.Itoa(len(lines)) + " lines of code."
			}
			return code
		}
	case agent.RoleResearcher:
		if notes, ok := data["researcher_notes"].(map[string]any); ok {
			if summary, ok := notes["summary"].(string); ok {
				return summary
			}
		}
		if summary, ok := data["analysis_summary"].(string); ok {
			return summary
		}
	case agent.RoleShadowAuditor:
		if found, ok := data["vulnerability_found"].(bool); ok {
			if found {
				backstory := ""
				if bs, ok := data["backstory"].(string); ok {
					backstory = bs
				}
				return "🚨 Vulnerability identified! Backstory: " + backstory
			}
			return "No vulnerabilities found in this sweep."
		}
	}

	return "Processed mission data."
}
