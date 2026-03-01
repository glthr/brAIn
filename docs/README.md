# Documentation index

All docs in one place. Start from the [main README](../README.md) "If you want to…" table for guided navigation. Quick links: [Why brAIn](overview.md) · [CLI reference](cli.md) · [Configuration](configuration.md).

## Concepts

| Doc | Description |
|-----|-------------|
| [overview.md](overview.md) | Why brAIn: one file, self-describing, agent-agnostic, consolidation, semantic search |
| [memory-model.md](memory-model.md) | How the five memory types map to the brain (working, episodic, semantic, procedural, social) |
| [lifecycle.md](lifecycle.md) | From short-term to long-term: working → episodic → semantic, consolidation phases |

## Usage and flow

| Doc | Description |
|-----|-------------|
| [agent-loop.md](agent-loop.md) | How an agent uses the brain each turn (remember, think, act, learn) |
| [ingestion.md](ingestion.md) | How raw conversation turns become memories (fast capture + LLM extraction) |

## Social memory

| Doc | Description |
|-----|-------------|
| [social-memory.md](social-memory.md) | Contacts, interaction log, trust scoring (agents), person schema (humans) |

## Integration

| Doc | Description |
|-----|-------------|
| [editors.md](editors.md) | AI editor integration: Cursor, Claude Code, Codex, Gemini CLI; what gets installed |
| [cursor.md](cursor.md) | Cursor integration: challenges and limitations (hooks not fully operational with Cursor CLI, etc.) |
| [adding-a-new-agent.md](adding-a-new-agent.md) | Checklist to add brAIn support to a new AI agent |

## Operations

| Doc | Description |
|-----|-------------|
| [daemon.md](daemon.md) | Background daemon: consolidation, encoding, profile analysis; config and service install |
| [configuration.md](configuration.md) | Config file, env vars, metadata, compact, logging |
| [llm.md](llm.md) | LLM and embedding config; when an LLM is required |
| [backup-restore.md](backup-restore.md) | Backup, restore, and move the brain to another machine |
| [troubleshooting.md](troubleshooting.md) | Common problems and FAQ |

## Reference

| Doc | Description |
|-----|-------------|
| [cli.md](cli.md) | Full CLI command reference and examples |
| [reference.md](reference.md) | Tests, example, project layout, CLI vs daemon |
| [sqlite.md](sqlite.md) | Tables, schema, migrations |

## Comparison

| Doc | Description |
|-----|-------------|
| [vs-static-instructions.md](vs-static-instructions.md) | brAIn vs static instruction files (CLAUDE.md, AGENTS.md, etc.) |
