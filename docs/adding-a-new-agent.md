# Adding a New Agent

This document is the canonical checklist for extending brAIn support to a new AI agent (e.g. another CLI, IDE, or chat client). Follow these steps so the brain protocol is installed and documented consistently.

You can also store this as a procedure in the brain for discoverability: `brain procedure --name add_brain_agent --desc "Steps to add brAIn support to a new agent" --steps "1. Choose agent id... 2. Identify where the agent loads instructions..."` so that `brain skill "add new agent"` or `brain skill "add brain support to new agent"` surfaces it.

## 1. Choose agent id

Pick a short, lowercase identifier for the agent (e.g. `gemini`, `cursor`, `codex`, `claude-code`). This value is used in every `--agent <id>` flag so that memories are attributed correctly.

## 2. Identify where the agent loads instructions

Find the path(s) the agent reads for global or project-level instructions. Examples:

- **Cursor:** `~/.cursor/rules/*.mdc` (global rules; optional hooks in `~/.cursor/hooks/`)
- **Claude Code:** `~/.claude/CLAUDE.md` (global) and `~/.claude/skills/*.md` (skills)
- **Codex:** `~/.codex/AGENTS.md` (global) and `~/.agents/skills/brain/SKILL.md` (skill)
- **Gemini CLI:** `~/.gemini/GEMINI.md` (global context file)

Note whether the agent supports a global file (active in every project) or only a project-level file (e.g. `GEMINI.md` in the project root). If only project-level, document that users can copy or symlink from the installed path.

## 3. Create protocol file(s) under `integrations/<agent>/`

- Create the directory: `integrations/<agent>/` (and subdirs if needed, e.g. `integrations/codex/skills/brain/`).
- Copy the brain protocol from an existing agent (e.g. `integrations/codex/AGENTS.md` or `integrations/claude/CLAUDE.md`).
- **Replace** every `--agent <existing>` with `--agent <new-id>`.
- **Adapt** tool/execution instructions to the agent’s wording:
  - Cursor: "execute using the `run_terminal_cmd` tool"
  - Codex / Gemini CLI: "execute in the shell (terminal)"
  - Claude Code: "execute using the appropriate tool (terminal commands)"
- Keep the same protocol structure: CLI reference (Read/Write), Protocol (START: one command `brain context [--topics "..."]`; AFTER each response: one command `brain encode`), and Notes.

Use the same file name the agent expects (e.g. `GEMINI.md` for Gemini CLI, `AGENTS.md` for Codex, `CLAUDE.md` for Claude Code).

## 4. Update the Makefile

Integration install/update/uninstall is handled by **`scripts/install-integrations.sh`**. The Makefile only calls it (`scripts/install-integrations.sh install|update|uninstall`). **Integrations are only installed when the agent is present** (config dir exists or command in PATH). To add a new agent:

- **PAIRS** in the script: add a line
  `"<agent-id>:integrations/<agent>/<file>:$HOME/.<agent-dir>/<file>"`
- **UNINSTALL_DESTS**: add a line  
  `"$HOME/.<agent-dir>/<file>"`
- **agent_present()**: add a `case` branch for `<agent-id>` (e.g. `gemini) command -v gemini >/dev/null 2>&1 ;;`).

If the agent needs special handling (e.g. chmod or sed like Cursor hooks), add a conditional block in the script that calls a small helper (like `install_cursor_hooks`) when that agent is present.

## 5. Update docs/editors.md

- Add a short section for the new agent (e.g. "## Gemini CLI") describing where brAIn installs the protocol and how the agent loads it (global vs project-level, and any fallback for project-only setups).
- Add one row per installed file to the **"What gets installed"** table: source path in repo (e.g. `integrations/gemini/GEMINI.md`) and target path (e.g. `~/.gemini/GEMINI.md`), plus a brief purpose.
- Optionally update the intro sentence that lists supported agents (e.g. "Cursor, Claude Code, Codex, and Gemini CLI").

## 6. (Optional) Update README.md

If the README lists supported agents (e.g. in "Quick start" or "Key features"), add the new agent name (e.g. "Gemini CLI") next to the others.

---

**Summary:** Choose id → find config path(s) → add `integrations/<agent>/` with protocol (correct `--agent` and execution wording) → Makefile install/update/uninstall → docs/editors.md section and table → README if applicable.
