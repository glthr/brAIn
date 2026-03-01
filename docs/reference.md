# Reference

## Pre-commit hook

To run strict lint, `go vet`, and tests before each commit:

```bash
make install-hooks
```

Requires [golangci-lint](https://golangci-lint.run/) on your PATH (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`). The hook runs `make lint-strict`, `go vet ./...`, and `make test`.

## Running Tests

```bash
make test
```

57 tests cover all memory subsystems, every consolidation phase, conversation ingestion, brain metadata, profile inference, semantic similarity search, and logging.

## Running the Example

```bash
go run ./cmd/example/
```

This creates an `agent_brain.brain` file in the current directory and demonstrates every subsystem: storing and querying all five memory types, recording A2A and A2H interactions, running a consolidation cycle, and inspecting brain metadata. The file is a regular SQLite database and can be opened with any SQLite tool.

**Procedural memory** is populated by explicit `brain procedure` commands or by consolidation/encoding (emerging skills). A fresh brain has 0 procedures until one of these runs; the example adds two procedures explicitly so all five memory types are visible.

## Two Tools: CLI and Daemon

brAIn provides two separate executables that work together:

### `brain` (CLI Tool)

The `brain` CLI is an interactive command-line tool for direct access to memory subsystems. It runs commands synchronously and exits immediately after completion. The CLI can operate without an LLM connection for read-only operations (querying memories, listing contacts, etc.), but requires an LLM for operations like `process-encodings`, `consolidate`, and `analyze-profiles`.

**Use the CLI for:**
- Interactive queries and manual operations
- One-off tasks (store a fact, recall memories, check activity)
- Scripts and automation that need immediate results
- Operations that don't need to run continuously

### `brain-daemon` (Background Service)

The `brain-daemon` is a long-lived background process that runs periodic maintenance tasks automatically. It **requires an LLM connection** and is the primary way LLM-powered operations (encoding extraction, consolidation, profile analysis) happen automatically in the background.

**Use the daemon for:**
- Automatic memory consolidation and semanticization
- Background processing of conversation encodings
- Continuous profile analysis for users
- Production deployments where maintenance should happen automatically

**Relationship:** Both tools operate on the same brain file (`~/.brain/agent.brain`). The CLI can perform any operation manually, while the daemon handles periodic maintenance automatically. You can run both simultaneously -- the CLI for interactive use and the daemon for background processing.

## Project layout

Main directories and entrypoints for contributors:

| Path | Purpose |
|------|---------|
| `cmd/brain/` | CLI entrypoint |
| `cmd/daemon/` | Daemon entrypoint; background consolidation, encoding, profiles |
| `cmd/example/` | Demo that exercises all memory subsystems |
| `internal/brain/` | Core Brain type and high-level operations (encode, consolidate) |
| `internal/store/` | SQLite access, schema, migrations |
| `internal/llm/` | LLM client, prompts, embedding, summarization, traits |
| `internal/daemon/` | Daemon config, runner, model defaults |
| `internal/memory/` | Working, episodic, semantic, procedural, social memory logic |
| `internal/consolidation/` | Consolidation phases (expire, decay, transfer, forget, semanticize) |
| `internal/helpers/` | Project resolution, tags, recall filtering |
| `integrations/` | Brain integrations (rules, hooks, skills) for Cursor, Claude Code, Codex, Gemini CLI |
| `scripts/` | Install hooks, integration install/uninstall |

Running tests and the pre-commit hook is covered above; see [Pre-commit hook](#pre-commit-hook) and [Running Tests](#running-tests).

## What to read next

- [Running the Example](#running-the-example) — see all subsystems in action.
- [Daemon](daemon.md) — configure and run background consolidation and encoding.
- [AI Editor Integration](editors.md) — Cursor and Claude Code setup.
