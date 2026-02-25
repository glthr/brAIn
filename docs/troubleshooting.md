# Troubleshooting and FAQ

## Troubleshooting

### Daemon fails to start

The daemon **requires an LLM connection** to run. If it exits immediately or logs an error on startup:

1. **Check LLM config:** Ensure `~/.brain/config` contains `llm_url` and `llm_model`, or set `BRAIN_LLM_URL` and `BRAIN_LLM_MODEL` in your environment. See [LLM Integration](llm.md) and [Daemon](daemon.md).
2. **Ollama not running:** If you use Ollama, start it first (`ollama serve` or the Ollama app). For a local Ollama URL (localhost:11434), the daemon will try to start `ollama serve` if not already running, but the required models must be available (the daemon will pull them on first run if missing).
3. **Required models missing:** The daemon ensures LLM, embed, and test models are available at startup. If a model pull fails (e.g. network error), the daemon exits. Fix the network or model name and try again.
4. **Logs:** Check `~/.brain/daemon.log` for the exact error message.

### CLI read-only works but encode / consolidate fails

Read-only commands (`brain recall`, `brain whoami`, `brain contacts`, etc.) do not need an LLM. Commands that use the LLM (`brain encode`, `brain process-encodings`, `brain consolidate`, `brain analyze-profiles`) require a working LLM connection.

- Configure `~/.brain/config` or `BRAIN_LLM_URL` and `BRAIN_LLM_MODEL` (and `BRAIN_LLM_API_KEY` if using a hosted API). See [LLM Integration](llm.md).
- If using Ollama, ensure it is running and the configured model is installed (`ollama list`).

### Agent says "we haven't met" or "user: unknown"

The brain identifies users from interaction history. On a fresh brain or before the first encoded conversation with a given person, there is no "current user."

- **Introduce yourself:** When the agent asks who you are, reply with your name (e.g. "I'm Guillaume"). The agent should then run `brain encode` (or record an interaction) so the brain associates your name with your user ID. After that, `brain whoami` and `brain context` will show you as the active user.
- **Check whoami:** Run `brain whoami` yourself; if it prints `unknown`, no human has been recorded yet. Use the agent (or `brain interact`) to record an interaction with your name and ID.

### `brain traits` shows "No profile yet" for a known user

Traits (person schema and behavioral priors) are inferred by the profiler from interaction history and memories. The LLM is instructed to return **empty** profile fields when there is insufficient data — for example, a new user with only one or two interactions and no long-term memories. So "No profile yet" is **expected** until enough conversations and memories exist. The profiler still runs and stamps the user as analyzed; on later cycles, when new data has accumulated, the LLM may then return non-empty traits. See [Social Memory — When traits are empty](social-memory.md#when-traits-are-empty-insufficient-data).

### Backup, restore, or move brain to another machine

See [Backup and restore](backup-restore.md) for step-by-step instructions.

### CGO / SQLite build errors

brAIn uses CGO to link the SQLite driver. If `go build` or `make` fails with CGO or SQLite errors:

- Ensure a C compiler is installed (e.g. Xcode Command Line Tools on macOS, `build-essential` on Debian/Ubuntu).
- Ensure CGO is enabled: `CGO_ENABLED=1` (usually the default when a C compiler is present).

---

## FAQ

### Can I use a different LLM than Ollama?

Yes. brAIn talks to any **OpenAI-compatible** API. Set `llm_url` to your endpoint (e.g. OpenAI, vLLM, LiteLLM) and `llm_model` to the model name. If the provider requires an API key, set `llm_api_key` in config or `BRAIN_LLM_API_KEY`. See [LLM Integration](llm.md).

### Where is data stored?

All data lives under **`~/.brain/`**. The main store is `~/.brain/agent.brain` (one SQLite file). Config is `~/.brain/config`, logs in `~/.brain/daemon.log` and `~/.brain/brain.log`, backups in `~/.brain/backups/`. See the [Data directory](../README.md#data-directory-brain) table in the README.

### How do I reset or start fresh?

- **Keep config, reset memory:** Remove or rename `~/.brain/agent.brain`, then run `brain init` to create a new empty brain. Your `config` and logs stay.
- **Full reset:** Remove the entire `~/.brain` directory (or rename it). Run `brain init` (or `make install`); a new directory and brain will be created.

### Why does the agent run `brain context` at the start of every turn?

The brain protocol (used by Cursor, Claude Code, Codex, Gemini CLI) instructs the agent to run **one** command at conversation start — `brain context [--topics "..."]` — to load who the user is, active goals, recalled facts and episodes for the topic, and traits. That way the agent has persistent context without you re-pasting it. See [AI Editor Integration](editors.md) and [Agent inference loop](agent-loop.md).
