# Daemon

The `brain-daemon` binary is a separate executable that runs as a long-lived background process. Unlike the `brain` CLI which runs commands and exits, the daemon continuously performs periodic maintenance tasks. It **requires an LLM connection** to function and is the primary mechanism for automatic memory processing.

| Task | Default interval | What it does |
|---|---|---|
| **Encoding processing** | 5 min (half of consolidation) | Runs `ProcessEncodings` -- extracts memories from raw conversation turns via LLM |
| **Consolidation** | 10 min | Runs the 5-phase consolidation cycle (expire, decay, transfer, forget, semanticize) |
| **Profile analysis** | 30 min | Runs `AnalyzeProfiles` -- infers person schema for users with new data |

## Configuration

The daemon reads `~/.brain/config`. All required Ollama model names (LLM, embed, and optional test model) are defined in one place in code (`internal/daemon/models.go`); config and environment override those defaults. On startup, when using an Ollama endpoint (any URL with port 11434), the daemon ensures every required model is available (pulling any that are missing). If any required model cannot be ensured (e.g. download failure), the daemon exits with an error. For a local Ollama URL (localhost or 127.0.0.1:11434), the daemon will also start `ollama serve` if not already running.

```ini
# How often the daemon runs memory consolidation.
consolidation_interval = 10m

# How often the daemon checks for users needing LLM-powered profile analysis.
profile_interval = 30m

# LLM connection for consolidation, profile inference, and conversation encoding.
# REQUIRED: The daemon requires LLM configuration to function.
llm_url = http://localhost:11434
llm_model = qwen3:4b
# llm_api_key =

# Optional: for daemon auto-update (build from source and restart)
# source_path = /path/to/brAIn
# auto_update_on_start = false
```

`brain init` creates this file with defaults. The daemon uses the same LLM settings and [environment variables](llm.md#environment-variables) as the CLI (see [LLM Integration](llm.md)). Additional env vars: `BRAIN_SOURCE_PATH`, `BRAIN_AUTO_UPDATE_ON_START`. Environment variables override config file values.

**Important:** LLM configuration (`llm_url` and `llm_model`) is **required for the daemon**. The daemon will fail to start if these are not configured.

## Running the daemon

The daemon can be run in two ways:

**Option 1: Run directly** (for testing or one-off runs)
```bash
brain-daemon
```
This runs the daemon in the foreground until interrupted. Useful for debugging or testing.

**Option 2: Install as a system service** (recommended for production)
```bash
brain install-service     # installs and starts the daemon
brain uninstall-service   # stops and removes the daemon
```
On macOS, `install-service` creates a launchd plist at `~/Library/LaunchAgents/com.brain.daemon.plist` with `KeepAlive` and `RunAtLoad` enabled. On Linux, it creates a systemd user service at `~/.config/systemd/user/brain-daemon.service`. In both cases the daemon **starts automatically when you open a session** (log in); you do not need to start it manually. Logs are written to `~/.brain/daemon.log`.

### Log rotation

- **Daemon** (`~/.brain/daemon.log`): Size-based rotation when the file exceeds 10 MB (default). Rotated files are renamed `daemon.log.YYYYMMDD-HHMMSS`; only the 5 most recent backups are kept. Override with `BRAIN_DAEMON_LOG_MAX_MB` and `BRAIN_DAEMON_LOG_MAX_BACKUPS`.
- **CLI** (`~/.brain/brain.log`): Uses lumberjack (max 10 MB per file, 2 backups, 7 days). See [Configuration](configuration.md#logging).

`make install` automatically runs `install-service` after building.

## Auto-update from source

The daemon can compile the latest binary from the brAIn repo and replace itself, then exit so the service restarts with the new code.

| Config (in `~/.brain/config`) | Purpose |
|------------------------------|---------|
| `source_path = /path/to/brAIn` | Repo root; daemon runs `go build -o <binary> ./cmd/daemon/` here. Env: `BRAIN_SOURCE_PATH`. |
| `auto_update_on_start = true` | On each start, build and replace the binary, then exit so launchd/systemd restarts the new binary. Env: `BRAIN_AUTO_UPDATE_ON_START`. |

**On demand:** Send `SIGUSR2` to the daemon (e.g. `brain daemon-update`), or `kill -SIGUSR2 $(pgrep -f brain-daemon)` on Linux. The daemon builds from `source_path`, replaces `~/bin/brain-daemon`, and exits; the service then restarts with the new binary.

## When to use CLI vs Daemon

- **Use the CLI** (`brain`) when you need immediate, interactive access to memories or want to perform one-off operations. The CLI can work without an LLM for read-only queries.

- **Use the daemon** (`brain-daemon`) when you want automatic background processing. The daemon handles periodic consolidation, encoding extraction, and profile analysis without manual intervention. It requires an LLM connection.

- **Use both**: Run the daemon as a service for automatic maintenance, and use the CLI for interactive queries and manual operations. Both can access the same brain file simultaneously.

### Pending work

`brain activity` shows how many conversation encodings are waiting for LLM extraction and how many users are queued for profile analysis. The daemon processes these on its schedule (encoding every 5 min, profile every 30 min). To process pending encodings immediately without waiting, run `brain process-encodings` (requires LLM). If the daemon was stopped during an encoding run, some encodings may remain pending until the next run or until you run `brain process-encodings`.
