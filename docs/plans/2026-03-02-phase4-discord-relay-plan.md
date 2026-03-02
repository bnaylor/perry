# Implementation Plan: Phase 4 — Discord Relay & Command Interface

This plan describes the phased rollout of the Discord sidecar and bi-directional event bus.

## Milestone 1: SQLite Schema & Storage Layer (Outbound)
Update the storage layer to support cursor tracking and command storage.

- [ ] **Task 1.1: Database Migrations**
    - Create `internal_state` table: `key TEXT PRIMARY KEY, value TEXT`.
    - Create `commands` table: `id INTEGER PRIMARY KEY, command TEXT, args TEXT, status TEXT, task_id TEXT, created_at TIMESTAMP`.
- [ ] **Task 1.2: Store Methods**
    - `GetInternalState(key string)` / `SetInternalState(key, value string)`.
    - `GetPendingCommands()` / `UpdateCommandStatus(id, status string)`.
- [ ] **Task 1.3: Thread Mapping**
    - Add `discord_thread_id` column to `tasks` table.
    - Update `Store.Create` and `Store.Update` to handle the thread ID.

## Milestone 2: Internal Discord Package
Encapsulate the Discord SDK and identity management.

- [ ] **Task 2.1: `internal/discord` Scaffold**
    - Wrapper around `github.com/bwmarrin/discordgo`.
    - `Client` struct that manages a pool of `*discordgo.Session`.
- [ ] **Task 2.2: Identity Pool**
    - Implementation of bot multiplexing: `SendAs(role agent.Role, channelID, message string)`.
- [ ] **Task 2.3: Message Formatting**
    - `text/template` based formatters for state transitions and audit findings.

## Milestone 3: The Relay Sidecar (`perry-relay`)
Create the standalone poller binary.

- [ ] **Task 3.1: `cmd/perry-relay` Scaffold**
    - Config loading for `configs/discord.yaml`.
    - Signal handling for graceful shutdown.
- [ ] **Task 3.2: Outbound Poller**
    - Loop that queries `transitions` and `agent_calls` where `id > cursor`.
    - Dispatches to `internal/discord` for posting.
    - Handles thread creation on `SUBMITTED`.
- [ ] **Task 3.3: Inbound Listener**
    - Discord message handler that filters for `!` commands.
    - Writes valid commands to the `commands` table.

## Milestone 4: Orchestrator Integration (Inbound)
Wire the Orchestrator to respond to Discord commands.

- [ ] **Task 4.1: Command Poller**
    - Add a background goroutine to `Orchestrator.Step` loop (or separate) to check for pending commands.
- [ ] **Task 4.2: Command Handlers**
    - Implement `!status`, `!pause`, `!resume`.
    - Wire `!approve` to advance `HUMAN_REVIEW` states.

## Milestone 5: Verification
- [ ] **Task 5.1: End-to-End Test Flight**
    - Submit a task via CLI.
    - Verify thread creation by Major Monogram.
    - Verify updates from Phineas, Ferb, Carl, and Doof.
    - Verify control via `!status` in Discord.

## Definition of Done
- `perry-relay` runs as a separate process.
- All task-related logs are confined to Discord threads.
- Agents use themed identities correctly.
- Orchestrator can be queried/controlled via Discord commands.
- `docs/local_llm_reference.md` updated with Relay setup instructions.
