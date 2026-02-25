# Cursor integration: challenges and limitations

brAIn integrates with Cursor via a **rule** (`brain.mdc`) and, in the **Cursor IDE only**, optional **hooks** for encoding. This document explains the limitations and design choices that come from Cursor’s current behavior, as reflected in the brAIn source and rule comments.

## Summary: what works where

| Context | Load context | Encode the turn |
|--------|---------------|------------------|
| **Cursor IDE** | Agent runs `brain context` (rule). Hook output from `beforeSubmitPrompt` is **not** injected into the agent. | **Hooks** (if installed): `beforeSubmitPrompt` saves prompt/user; `stop` runs `brain encode`. Rule still tells the agent to run `brain encode` when hooks don’t fire. |
| **Cursor CLI** | Agent runs `brain context` (rule). | **Agent only.** The CLI does **not** fire `stop` or `beforeSubmitPrompt`; the agent must call `brain encode` after every response. |

So: **context is always agent-driven** in Cursor; **encoding** is hook-driven in the IDE (when hooks are installed) and **must be agent-driven in the CLI**.

## 1. Cursor CLI: lifecycle hooks are not implemented

The Cursor CLI only fires hooks for **resource operations**, not for conversation lifecycle:

- **Implemented:** `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `afterFileEdit`
- **Not implemented:** `stop`, `beforeSubmitPrompt`, `afterAgentResponse`, `afterAgentThought` — these exist in the type system but have **no caller** in the CLI’s agent loop; they are never fired.

As a result:

- **Context** cannot be injected by a hook in the CLI; the agent must run `brain context` at the start of each turn (as the rule instructs).
- **Encoding** cannot be done by a hook in the CLI; the agent must run `brain encode` at the end of each turn. The rule explicitly states: *“The Cursor CLI does not fire `stop` or `beforeSubmitPrompt` hooks — the agent must call `brain encode` itself after every response.”*

The install script reflects this: for Cursor it installs the rule and the memory subagent only; encode is **not** delegated to hooks in the CLI because those hooks are never run. See `scripts/install-integrations.sh`: *“Cursor: subagent only … Encode is handled by the agent via the rule, not by hooks — the Cursor CLI does not fire stop or beforeSubmitPrompt hooks.”*

## 2. Cursor IDE: hook output is not injected as context

Even in the Cursor IDE, **stdout from `beforeSubmitPrompt` is not injected into the agent’s context**. So we cannot use a hook to run `brain context` and have the editor feed that output to the model (unlike Claude Code, which does inject hook output). In Cursor, the agent must run `brain context` itself so that the tool result delivers brain output in the same turn.

## 3. Rule attachment: `alwaysApply` can be unreliable

The rule uses `alwaysApply: true` so Cursor injects it into every conversation. In some Cursor versions this is not reliable (see [forum discussion](https://forum.cursor.com/t/rule-file-with-alwaysapply-enabled-not-added-automatically/92856/2)). The rule therefore also sets `globs: ["**/*"]` so it attaches when any project file is in context. If the rule still does not load, the user can add it explicitly (e.g. `@brain.mdc`). This is noted in the rule’s top-level comment.

## 4. Stop hook does not receive the agent’s reply

When the **stop** hook runs in the Cursor IDE, Cursor does **not** pass the assistant’s reply — only `generation_id`, `status`, and similar metadata. So the encode script cannot send the real response to `brain encode`. It uses a **placeholder** for `--response` (e.g. “(response captured via Cursor stop hook — full transcript in Cursor)”). The brain still gets user id, prompt, and metadata; consolidation and extraction can use the stored interaction, but the stored text of the assistant message is not the actual reply. If Cursor ever exposes the assistant’s reply to the stop hook, the integration could pass the real response.

## 5. Why two hooks in the IDE (beforeSubmitPrompt + stop)

We need the **user’s prompt** and **current user id** to call `brain encode`. Cursor does not re-send the prompt or the agent’s reply to the stop hook. So we capture prompt and user at the only time they are available: when the user submits. The **beforeSubmitPrompt** hook runs `brain whoami`, parses id/name from the output (or uses `unknown` if whoami fails), and writes `prompt`, `user_id`, and `user_name` to a temp file keyed by `generation_id`. The **stop** hook reads that file and runs `brain encode`. Two hooks are required by Cursor’s API: one to capture input, one to run the side effect after the turn.

## 6. Single rule, no separate skill file

Other brAIn integrations (e.g. Claude Code, Codex) install a **skill file** alongside global instructions. In Cursor we do **not** install a separate skill: the abstraction is **rules**, and the full protocol lives in one rule file (`brain.mdc`) with `alwaysApply: true`. The rule documents both “run `brain context` at start” and “run `brain encode` after your response” so the same instructions work in the IDE (with or without hooks) and in the CLI, where hooks are not used for encode.

## Practical implications

- **Using Cursor CLI:** Rely on the rule only. The agent must run `brain context` once at the start of each turn and `brain encode` at the end; no hooks are involved.
- **Using Cursor IDE:** You can install hooks so that encoding is done by the stop hook when the IDE fires it; the rule still tells the agent to run `brain encode` so behavior is correct when hooks don’t run or in CLI.
- **Rule not loading:** Try `globs: ["**/*"]` (already in the rule) or reference the rule explicitly (e.g. `@brain.mdc`).

For the exact hook contracts (JSON payloads, data flow, script paths) when using the IDE with hooks, see [AI Editor Integration](editors.md): [Cursor hooks contract](editors.md#cursor-hooks-contract).
