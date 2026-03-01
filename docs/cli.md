# CLI Tool

The `brain` CLI provides direct access to every memory subsystem. The brain file is always `~/.brain/agent.brain` — no path argument needed.

## Commands

| Command | Description |
|---|---|
| `init` | Create the brain file (if it doesn't exist) |
| `encode` | Encode a conversation turn (fast write, LLM extracts later) |
| `fact` | Store a fact in semantic memory |
| `recall` | Look up facts by keyword (scoped to current project by default) |
| `episode` | Record an episodic memory |
| `episodes` | List recent episodes (optionally filter by tag) |
| `working` | Store a working memory item |
| `attention` | Show active attentional focus (goals first, then working memory) |
| **`context`** | **One-shot start-of-turn context:** whoami, attention, recall by topics, episodes, traits. This is the main command agents run at the start of each turn. Optional: `--topics "topic1,topic2"` to recall by topic. |
| `context-budget` | Return ranked, deduplicated memories for a topic within a token budget (`--topic`, `--tokens`) |
| `goal` | Create a persistent goal (survives until resolved) |
| `resolve-goal` | Mark a goal as completed and remove it |
| `timeline` | Show archived episodic memories (narrative history before semanticization) |
| `forget` | Hard-delete a specific memory by ID |
| `pin` / `unpin` | Pin a memory (salience=1.0, exempt from decay) or remove pin |
| `correct` | Update memory content in-place (preserves history) |
| `explain` | Trace a memory's lifecycle (source, salience, pinned status) |
| `procedure` | Store a new procedure |
| `procedures` | List learned procedures |
| `skills` | List emerging skills (procedures learned from conversation or consolidation) |
| `skill` | Search procedures by goal/intent (semantic similarity); returns matching procedures with steps so the agent can apply one |
| `interact` | Record an interaction (agent or human) |
| `whoami` | Show the most recent human user |
| `traits` | Show the person schema and behavioral priors for a contact |
| `profile` | Show full profile (info + person schema + behavioral priors) for a contact |
| `memories` | List memories associated with a user |
| `needs-analysis` | List users with new data since last analysis |
| `mark-analyzed` | Stamp a user as analyzed (used internally by the profiler) |
| `contacts` | List known contacts (optionally filter by type) |
| `history` | Show interaction history for a contact |
| `analyze-profiles` | Run LLM-powered profile inference for all users with new data |
| `process-encodings` | Process raw conversation turns through LLM extraction |
| `consolidate` | Run the consolidation sleep cycle |
| `install-service` | Install daemon as a system service (launchd on macOS, systemd on Linux) |
| `uninstall-service` | Remove the daemon system service |
| `daemon-status` | Show whether the daemon is running and the last 15 lines of `~/.brain/daemon.log` |
| `daemon-restart` | Restart the daemon service (no rebuild) |
| `daemon-update` | Ask the daemon to rebuild from source and restart (sends SIGUSR2; requires `source_path` in config) |
| `activity` | Show recent brain activity (daemon health check) |
| `config` | Show current config values and file location |
| `stats` | Show brain statistics |
| `meta` | Show or set metadata |
| `backup` | Copy brain to a timestamped file in `~/.brain/backups/` and set `last_backup_at` |
| `validate` | Run read-only checks (schema, brain_meta, table row counts) |

**Procedures vs skills:** `brain procedures` lists all procedures (explicit + emerging). `brain skills` lists emerging skills (procedures learned from conversation or consolidation). `brain skill <query>` searches procedures by goal or intent and returns matching steps so the agent can apply one.

**Deprecated aliases** (still work): `ingest` → `encode`, `process-ingests` → `process-encodings`.

## Examples

```bash
# Create a brain
brain init

# Encode conversation turns (fast, no LLM needed)
brain encode --user user-alice --name Alice --agent cursor --prompt "How do I test Go?" --response "Use table-driven tests..."

# Store and recall facts
brain fact --tags go,concurrency Go channels are typed conduits
brain fact --tags go,concurrency --user user-alice Goroutines blocked on nil channel block forever
brain recall goroutine          # scoped to current project + common
brain recall --project /path/to/project goroutine
brain recall --all goroutine

# Record experiences
brain episode --tags debugging,go --user user-alice Debugged deadlock using pprof

# Track working context
brain working --user user-alice Currently reviewing PR #42

# Store a procedure
brain procedure --name code_review --steps "read diff,check tests,comment"

# List emerging skills (learned from conversation or consolidation)
brain skills

# Search for a procedure by goal (e.g. to apply an emerging skill)
brain skill "convert to PDF"

# Record interactions with agents and humans
brain interact --id bot-search --name SearchBot --entity agent --outcome success
brain interact --id user-alice --name Alice --entity human --outcome success \
    --notes "Asked about goroutine patterns, resolved quickly"

# Identify the current user
brain whoami

# View person schema, behavioral priors, and full profile
brain traits user-alice
brain profile user-alice

# List memories associated with a user
brain memories user-alice

# List contacts (all, or filtered by type)
brain contacts
brain contacts --type agent
brain contacts --type human

# View interaction history for a specific contact
brain history user-alice
brain history bot-search --limit 10

# Process ingested conversations through LLM
brain process-encodings

# Run LLM-powered profile inference
brain analyze-profiles

# Run consolidation (decay, prune, summarize)
brain consolidate

# Run the background daemon (separate binary, reads ~/.brain/config)
brain-daemon
# Or install it as a system service (recommended):
brain install-service
brain uninstall-service
brain daemon-status

# Start-of-turn context (what agents run each turn)
brain context
brain context --topics "go,testing"

# Token-bounded context for a topic
brain context-budget --topic "authentication" --tokens 2000

# View config
brain config

# Health check
brain activity

# Inspect
brain stats
brain meta
brain meta description="My personal brain"
```

## Integration patterns

Because the CLI is a simple unix tool, any agent can use it:

- **Shell scripts**: wrap `brain recall` and `brain fact` in a script that feeds context to an LLM.
- **Cron**: schedule `brain consolidate` to run nightly (or use the built-in daemon).
- **Subprocess**: call `brain` from Python, Node, or any language via `exec`.
- **Pipes**: `brain recall goroutine` outputs plain text, easy to parse.
