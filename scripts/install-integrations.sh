#!/usr/bin/env bash
# Install, update, or uninstall brAIn integration files per agent.
# Only installs/updates when the agent is present (config dir or command in PATH).
# Install and update both overwrite existing files in $HOME/.cursor/... (and other
# agent dirs) so the installed copy always matches the repo.
# Update mode reinstalls integrations for every agent currently present, so if you
# install a new agent (e.g. Gemini CLI), running "make update" adds brain support for it.
# Usage: install-integrations.sh install | update | uninstall

set -e

MODE="${1:?Usage: install-integrations.sh install|update|uninstall}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOME="${HOME:-$(eval echo ~)}"

# agent:source:dest (source relative to REPO_ROOT). Add new agents here and to UNINSTALL_DESTS.
# Cursor also installs the memory subagent via install_cursor_extra below. No hooks: the Cursor CLI
# does not fire stop or beforeSubmitPrompt hooks, so encoding is done by the agent per the rule.
# Claude Code also installs hooks via install_claude_hooks below.
PAIRS=(
	"cursor:integrations/cursor/rules/brain.mdc:$HOME/.cursor/rules/brain.mdc"
	"claude:integrations/claude/CLAUDE.md:$HOME/.claude/CLAUDE.md"
	"claude:integrations/claude/skills/brain.md:$HOME/.claude/skills/brain.md"
	"codex:integrations/codex/AGENTS.md:$HOME/.codex/AGENTS.md"
	"codex:integrations/codex/skills/brain/SKILL.md:$HOME/.agents/skills/brain/SKILL.md"
	"gemini:integrations/gemini/GEMINI.md:$HOME/.gemini/GEMINI.md"
)

UNINSTALL_DESTS=(
	"$HOME/.claude/CLAUDE.md"
	"$HOME/.claude/skills/brain.md"
	"$HOME/.claude/hooks/brain-context.sh"
	"$HOME/.claude/hooks/brain-encode.sh"
	"$HOME/.cursor/rules/brain.mdc"
	"$HOME/.codex/AGENTS.md"
	"$HOME/.agents/skills/brain/SKILL.md"
	"$HOME/.gemini/GEMINI.md"
)

agent_present() {
	local agent="$1"
	case "$agent" in
		cursor) [ -d "$HOME/.cursor" ] || command -v cursor >/dev/null 2>&1 ;;
		claude) [ -d "$HOME/.claude" ] || command -v claude >/dev/null 2>&1 ;;
		codex)  [ -d "$HOME/.codex" ] || command -v codex >/dev/null 2>&1 ;;
		gemini) command -v gemini >/dev/null 2>&1 ;;
		*)      false ;;
	esac
}


# Cursor: subagent only (agents/memory.md). Encode is handled by the agent via the rule,
# not by hooks — the Cursor CLI does not fire stop or beforeSubmitPrompt hooks.
install_cursor_extra() {
	mkdir -p "$HOME/.cursor/agents"
	cp -f "$REPO_ROOT/integrations/cursor/agents/memory.md" "$HOME/.cursor/agents/memory.md"
}

# Claude Code: install hook scripts and merge hook config into ~/.claude/settings.json
install_claude_hooks() {
	mkdir -p "$HOME/.claude/hooks"
	cp -f "$REPO_ROOT/integrations/claude/hooks/brain-context.sh" "$HOME/.claude/hooks/brain-context.sh"
	cp -f "$REPO_ROOT/integrations/claude/hooks/brain-encode.sh"  "$HOME/.claude/hooks/brain-encode.sh"
	chmod +x "$HOME/.claude/hooks/brain-context.sh" "$HOME/.claude/hooks/brain-encode.sh"

	# Merge brain hooks into ~/.claude/settings.json, preserving existing keys
	python3 - "$HOME" <<'PYEOF'
import json, os, sys

home         = sys.argv[1]
settings_path = os.path.join(home, ".claude", "settings.json")

settings = {}
if os.path.exists(settings_path):
    try:
        with open(settings_path) as f:
            settings = json.load(f)
    except Exception:
        pass

settings["hooks"] = {
    "UserPromptSubmit": [
        {"hooks": [{"type": "command", "command": f"{home}/.claude/hooks/brain-context.sh"}]}
    ],
    "Stop": [
        {"hooks": [{"type": "command", "command": f"{home}/.claude/hooks/brain-encode.sh", "async": True}]}
    ],
}

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")
PYEOF
}

uninstall_claude_hooks() {
	# Remove brain hooks from ~/.claude/settings.json, preserving other keys
	python3 - "$HOME" <<'PYEOF'
import json, os, sys

home          = sys.argv[1]
settings_path = os.path.join(home, ".claude", "settings.json")

if not os.path.exists(settings_path):
    sys.exit(0)

try:
    with open(settings_path) as f:
        settings = json.load(f)
except Exception:
    sys.exit(0)

settings.pop("hooks", None)

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")
PYEOF
}

case "$MODE" in
	install|update)
		for pair in "${PAIRS[@]}"; do
			agent="${pair%%:*}"; rest="${pair#*:}"
			src="$REPO_ROOT/${rest%%:*}"; dest="${rest#*:}"
			if agent_present "$agent"; then
				mkdir -p "$(dirname "$dest")"
				cp -f "$src" "$dest"
				[ "$MODE" = "install" ] && echo "Installed $dest"
			fi
		done
		if agent_present "cursor"; then
			install_cursor_extra
			if [ "$MODE" = "install" ]; then
				echo "Installed Cursor subagent ~/.cursor/agents/memory.md"
			else
				echo "Updated Cursor subagent"
			fi
		fi
		if agent_present "claude"; then
			install_claude_hooks
			if [ "$MODE" = "install" ]; then
				echo "Installed Claude Code hooks to ~/.claude/hooks/ and ~/.claude/settings.json"
			else
				echo "Updated Claude Code hooks"
			fi
		fi
		if [ "$MODE" = "update" ]; then
			echo "Updated brain integration files"
		fi
		;;
	uninstall)
		for dest in "${UNINSTALL_DESTS[@]}"; do
			rm -f "$dest"
		done
		rm -f "$HOME/.cursor/agents/memory.md"
		uninstall_claude_hooks
		;;
	*)
		echo "Usage: install-integrations.sh install|update|uninstall" >&2
		exit 1
		;;
esac
