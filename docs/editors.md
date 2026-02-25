# AI Editor Integration

brAIn ships with skill files for **Cursor**, **Claude Code**, **Codex**, and **Gemini CLI**. `make install` copies them to the right locations so the brain protocol is active immediately in each environment. The skill files contain the full protocol: how to identify the user at the start of a conversation, how to recall relevant context, and how to encode each conversation turn into the brain.

## Cursor

Cursor supports project and global rules via `.mdc` files in `.cursor/rules/`. brAIn installs its rule to `~/.cursor/rules/brain.mdc` (global, active in every project).

The key feature is the **`alwaysApply: true`** frontmatter option:

```yaml
---
alwaysApply: true
---
```

When `alwaysApply` is set to `true`, Cursor injects the rule into every conversation automatically -- the user never has to reference it or opt in. This makes the brain protocol systematic: the agent runs **one command** at the start (`brain context [--topics "..."]` to load whoami, attention, recall, episodes, and traits) and **one command** after each response (`brain encode`). No human discipline required. (In some Cursor versions, `alwaysApply` alone may not inject the rule; the rule also sets `globs: ["**/*"]` so it attaches when any project file is in context.)

### Cursor CLI: rule-only (no hook support for encode)

The Cursor CLI does **not** fire `stop` or `beforeSubmitPrompt` hooks — those hook types have no caller in the CLI's agent loop. Only `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, and `afterFileEdit` are implemented. As a result, **both context loading and encoding are agent-driven** in the CLI:

- The rule instructs the agent to run `brain context` at the start of each turn.
- The rule instructs the agent to run `brain encode` at the end of each turn.

No hooks are required for Cursor CLI. `make install` still installs a **memory subagent** (`~/.cursor/agents/memory.md`) you can invoke with `/memory`.

### Remote rule from GitHub (alternative install)

If you prefer not to run `make install`, you can add the brain rule as a **Remote Rule** from GitHub so Cursor syncs it from the repo:

1. Open **Cursor Settings → Rules**.
2. Under Project Rules, click **Add Rule** → **Remote Rule (Github)**.
3. Paste the URL of the **raw** `brain.mdc` file, e.g.  
   `https://raw.githubusercontent.com/glthr/brAIn/main/integrations/cursor/rules/brain.mdc`
4. Cursor will pull and keep the rule synced. This is project-level; for a global rule you still need to copy to `~/.cursor/rules/brain.mdc` (e.g. via `make install`).

### Cursor hooks contract

brAIn uses two Cursor hooks. The following describes the JSON payloads so you can debug or adapt them.

**Hook events used**

| Event | Script | Purpose |
|-------|--------|---------|
| `beforeSubmitPrompt` | `save-prompt.sh` | Runs when the user sends a message. Saves the prompt and current user id to a temp file so the stop hook can encode the turn. |
| `stop` | `brain-encode.sh` | Runs when the agent completes a response. Reads the saved prompt/user, then runs `brain encode`. |

**Stdin JSON (both hooks)**

Cursor sends one JSON object per line on stdin. Fields we use:

| Field | Used in | Description |
|-------|---------|-------------|
| `generation_id` | both | Unique id for this turn. `save-prompt.sh` writes `/tmp/cursor-brain/{generation_id}.json`; `brain-encode.sh` reads it. |
| `prompt` | beforeSubmitPrompt | The user's message text. |
| `status` | stop | Completion status. We only encode when `status` is `"completed"`. |

Other fields (e.g. `conversation_id`, `hook_event_name`, `workspace_roots`, `attachments`) may be present; we ignore them. Exit 0 in all cases so Cursor continues.

**Data flow**

1. User sends a message → Cursor runs `beforeSubmitPrompt` with `generation_id` and `prompt`. The script runs `brain whoami`, parses id/name from the output (or uses `unknown` if whoami fails), then always writes `{ "prompt", "user_id", "user_name" }` to `/tmp/cursor-brain/{generation_id}.json`.
2. Agent replies → Cursor runs `stop` with `generation_id` and `status`. If `status == "completed"` and the temp file exists, the script reads it, finds `brain` on PATH or in `~/bin`, runs `brain encode` with the saved user id and name, and deletes the file.

**If hooks don’t run:** Ensure `make install` (or the Cursor install step) was run so `~/.cursor/hooks.json` and/or `~/.cursor/hooks/hooks.json` exist and point to the scripts, and that `~/.cursor/hooks/save-prompt.sh` and `brain-encode.sh` are executable. The `brain` CLI must be on your PATH or in `~/bin` when Cursor invokes the hooks. Restart Cursor after installing or updating hooks.

## How the agent uses brain data

The agent **leverages** brain info by running read commands at the start of each turn and receiving their **output as context** in the same turn (via the terminal / tool results). It does **not** modify or replace the user's message. What gets augmented is the **agent's context**, not the literal user prompt:

1. **Same turn:** The agent runs a single command, `brain context [--topics "topic1,topic2"]`, which returns whoami, attention, recall and episodes per topic, recent episodes, and traits. The results come back as tool output in that turn, so when the model generates a reply it "sees" both the user's message and the brain output.
2. **Uses that context to respond:** The skill instructs the agent to greet by name, reference past episodes and facts when relevant, and adapt style to the user's behavioral priors (from traits). So the **response** is informed by brain data; the **prompt** text the user sent stays unchanged.
3. **No prompt rewriting:** If you need to literally prepend recalled context to a single "prompt" string (e.g. for an API that accepts one prompt field), you would do that in your own integration (e.g. `context=$(brain recall X); prompt="$context\n\n$user_prompt"`). The Cursor/Claude skill does not rewrite the user prompt; it gives the agent extra context so it can answer with memory and personality in mind.

## Why the brain benefits the agent

Using the brain makes the agent more effective and consistent over time:

- **Continuity across sessions** — Each conversation is stateless by default. The brain gives the agent a persistent identity: it can greet the user by name, remember who they are, and pick up where a previous session left off (e.g. "Last time we were refactoring the auth module").
- **Reuse of past work** — Recalled facts and episodes let the agent avoid repeating mistakes, apply solutions that worked before, and reference prior decisions or preferences instead of asking again.
- **User-aware behavior** — Traits and profiles (inferred by the daemon from interaction history) let the agent adapt tone and detail level (e.g. concise for senior devs, more explanation for juniors) and respect stated preferences without hardcoding them in the system prompt.
- **Better answers with less in the prompt** — The user does not have to re-explain the project, stack, or context every time; the agent can pull relevant facts and recent episodes so answers are grounded in what it already "knows" about the project and the user.
- **Trust and outcome tracking** — For agents, the brain tracks trust scores; for humans, interaction history and outcomes. This helps the agent prioritize (e.g. which agent to delegate to) and adapt to users (person schema, priors).
- **Emerging skills (when supported)** — The consolidation prompt instructs the LLM to recognize patterns that reveal a successful procedure (goal + steps + success); once the pipeline stores these in procedural memory (see [Emerging skills](memory-model.md#emerging-skills-skills-that-emerge-from-interaction)), the agent can reuse user-taught or discovered how-tos (e.g. "convert to PDF") without a new table — the same procedural memory stores both explicit and emerging procedures.

Overall, the brain turns the agent from a stateless responder into one that accumulates experience and uses it to improve relevance, consistency, and rapport.

## Claude Code

Claude Code uses two files:

- **`~/.claude/CLAUDE.md`** -- Global instructions loaded into every conversation (analogous to Cursor's `alwaysApply`). brAIn installs a brief project overview here.
- **`~/.claude/skills/brain.md`** -- A skill file containing the full brain protocol. Claude Code can reference skills explicitly, but because the protocol is also in the global instructions, it is always active.

## Codex

Codex uses global instructions and a skill file:

- **`~/.codex/AGENTS.md`** -- Global instructions loaded into every conversation.
- **`~/.agents/skills/brain/SKILL.md`** -- Brain protocol skill (full protocol).

## Gemini CLI

Gemini CLI uses **GEMINI.md** as a custom context file. brAIn installs the brain protocol to **`~/.gemini/GEMINI.md`** so that Gemini CLI can load it as global context when you run `gemini` in the terminal. If your Gemini CLI setup only reads GEMINI.md from the current project directory, you can copy or symlink `~/.gemini/GEMINI.md` into your project as `GEMINI.md` so the brain protocol is still applied.

## What gets installed

| File | Target | Purpose |
|---|---|---|
| `integrations/cursor/rules/brain.mdc` | `~/.cursor/rules/brain.mdc` | Cursor global rule; agent runs `brain context` at start and `brain encode` after each response (CLI does not fire stop/beforeSubmitPrompt hooks) |
| `integrations/cursor/agents/memory.md` | `~/.cursor/agents/memory.md` | Cursor subagent (invoke with `/memory`) |
| `integrations/claude/CLAUDE.md` | `~/.claude/CLAUDE.md` | Claude Code global instructions |
| `integrations/claude/skills/brain.md` | `~/.claude/skills/brain.md` | Claude Code skill (full protocol) |
| `integrations/claude/hooks/brain-context.sh` | `~/.claude/hooks/brain-context.sh` | Claude Code hook: auto-run `brain context` on every prompt |
| `integrations/claude/hooks/brain-encode.sh` | `~/.claude/hooks/brain-encode.sh` | Claude Code hook: auto-run `brain encode` on every stop |
| `integrations/codex/AGENTS.md` | `~/.codex/AGENTS.md` | Codex global instructions |
| `integrations/codex/skills/brain/SKILL.md` | `~/.agents/skills/brain/SKILL.md` | Codex brain skill (full protocol) |
| `integrations/gemini/GEMINI.md` | `~/.gemini/GEMINI.md` | Gemini CLI brain instructions (global context) |

All of these files contain the same brain protocol — CLI reference, read/write commands, and the start/end-of-conversation protocol. The only difference is the delivery mechanism: Cursor uses `alwaysApply` frontmatter + hooks, Claude Code uses the global `CLAUDE.md` + lifecycle hooks, Codex uses `AGENTS.md` and the brain skill, and Gemini CLI uses `GEMINI.md`.
