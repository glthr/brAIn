# Why brAIn

LLM-based agents are stateless by default. Each conversation starts from zero. They have no persistent sense of what happened yesterday, what strategies worked, or which other agents are trustworthy. brAIn fixes this by giving an agent a structured, long-lived memory -- modeled after the way the human brain actually organizes information.

Everything lives in a single SQLite file with a `.brain` extension. No Redis, no Postgres, no external services. One `.brain` file that an agent can carry with it, back up, or share. The file is a standard SQLite database -- you can open it with any SQLite client (`sqlite3 ~/.brain/agent.brain`) for inspection or debugging.

By convention, brain files live in `~/.brain/`. The default brain is `~/.brain/agent.brain` -- a single global brain shared across all projects and AI tools (Claude Code, Cursor, CLI scripts). Every tool reads and writes to the same file, so knowledge, agent trust scores, and interaction history are unified regardless of which editor or agent framework you use.

## Advantages

### One file = one brain

The entire state of an agent's memory -- every fact, every experience, every agent trust score -- is a single `.brain` file. This has profound practical consequences:

- **Transplant.** Copy the file from machine A to machine B, point the agent at it, and the agent resumes exactly where it left off. No database migration, no service dependency, no network. `scp ~/.brain/agent.brain prod-server:~/.brain/` is a deployment.
- **Backup.** `cp ~/.brain/agent.brain ~/.brain/agent.brain.bak`. Done. Schedule it with cron. The brain records when it was last backed up (`last_backup_at` in the metadata table), so you can monitor freshness.
- **Version control.** Check the `.brain` file into Git LFS or a blob store. Roll back to last Tuesday's brain in one command.
- **Clone.** Want to fork an agent? Copy the file. Two agents now share the same starting memories but diverge independently from that point.
- **Inspect and debug.** The file is a standard SQLite database. Open it with `sqlite3`, DB Browser, or any SQLite client. Run ad-hoc queries. No opaque binary format, no proprietary lock-in.

### Self-describing

Every `.brain` file carries its own identity in a `brain_meta` key-value table:

| Key | Example value | Purpose |
|---|---|---|
| `schema_version` | `1` | Enables forward-compatible migrations |
| `created_at` | `2026-02-17 09:00:00` | When this brain was first created |
| `description` | `Production research assistant` | Free-form description (optional) |
| `last_backup_at` | `2026-02-17 12:30:00` | Timestamp of the most recent backup (optional) |

The keys `description` and `last_backup_at` are optional and may be absent until you set them (e.g. via `brain meta` or `brain init --desc`, and `brain backup` to record a backup). Only `schema_version` and `created_at` are always present.

When you pick up a `.brain` file you've never seen before, you can immediately answer: *When was this brain created? When was it last backed up?* This makes `.brain` files self-documenting artifacts that can be cataloged, audited, and safely migrated between systems. The brain is deliberately agent-agnostic -- it does not store which AI agent (Cursor, Claude Code, etc.) owns it, because any agent can read and write to it. Instead, each individual memory records which agent created it via the `agent` column.

### Agent-agnostic

The brain does not belong to any single AI agent. Cursor, Claude Code, or a CLI script can all read and write to the same `.brain` file -- knowledge, trust scores, and interaction history are unified. Each memory records which agent created it (via the `agent` column), so you can always trace provenance, but the brain itself is neutral. Switch editors mid-task and the new agent picks up right where the old one left off.

### LLM connection required for full functionality

See [Prerequisites](../README.md#prerequisites) for a concise summary. The brain links SQLite statically via CGO, so no database server is needed. For LLM operations (consolidation, encoding, profiling), configure an LLM in `~/.brain/config` or via environment variables; see [LLM Integration](llm.md) and [Daemon](daemon.md).

### Consolidation is built in

Most memory systems are append-only logs that grow without bound. brAIn includes a consolidation engine ("sleep cycle") that autonomously prunes, decays, promotes, and summarizes memories. The agent's own usage patterns determine what it remembers -- no manual curation required.

### Emerging skills from interaction

The brain does not only store facts and episodes; it **learns how to do things** from conversation and from what works. When you teach a procedure ("to convert to PDF I use LibreOffice: File → Export as PDF") or when the agent discovers a successful approach (e.g. fixing a goroutine leak with pprof), consolidation can turn that into a reusable **procedure** in procedural memory. The agent can then search by goal (`brain skill "convert to PDF"`) and apply the steps next time -- no new table, no manual recipe file; skills emerge from use.

### Semantic similarity search

Semantic, episodic, and procedural memory use embedding-based semantic similarity search. Keyword lookups find semantically related memories even when exact keywords don't match (e.g. "concurrency" can match memories about "goroutines"). Search is powered by embeddings generated via Ollama or any OpenAI-compatible API.

### Stores context with embeddings

brAIn stores what actually matters at inference time: conversations, reasoning traces, tool outputs, task outcomes, and learned facts -- as plain text with tags and metadata. When an embedding model is configured, content is automatically embedded and stored alongside memories for semantic search. This keeps the data human-readable, inspectable with `sqlite3`, while enabling powerful semantic similarity queries.
