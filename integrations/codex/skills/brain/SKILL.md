---
name: brain
description: Persistent memory system for AI agents. Use this skill to recall past context, store new knowledge, and maintain continuity across sessions via the brain CLI.
---

# brAIn -- Persistent Memory

You have a persistent memory system via the `brain` CLI. The brain file lives at `~/.brain/agent.brain` and is used automatically -- you never specify a path. Use the brain **systematically** -- at the start and end of every conversation -- to build long-term continuity across sessions.

The brain handles its own maintenance automatically: consolidation, profile inference, and memory extraction all happen behind the scenes via a local LLM.

## How it works

After each conversation turn, you send the user's prompt and your response to `brain encode`. The brain's LLM extracts facts, episodes, tags, and interaction metadata automatically. You don't need to decide what to store -- the brain figures it out.

## CLI reference

All commands: `brain <command> [flags] [args...]`

Flags come **before** positional text (Go convention).

### Read

**One-shot start-of-turn context (run once when receiving a prompt):**

```bash
brain context [--topics "topic1,topic2,topic3"]   # whoami + attention + recall + episodes + traits in one call
```

Extract 1–3 keywords from the user's message and pass as `--topics` (e.g. `--topics "authentication,bug"`). If omitted, context still runs whoami, attention, recent episodes, and traits.

Other read commands (for ad-hoc use):

```bash
brain whoami                                   # most recent human user
brain attention                                # active working memories
brain recall <keyword>                         # look up facts (scoped to current project by default; use --all for cross-project)
brain episodes --limit 5                       # recent episodes
brain episodes --tag <tag>                     # filter episodes by tag
brain procedures                               # list learned procedures
brain contacts                                 # all contacts
brain contacts --type human                    # only humans
brain traits <contact-id>                      # person schema & behavioral priors
brain profile <contact-id>                     # full profile (info + traits)
brain memories <contact-id>                    # memories associated with a user
brain activity                                 # recent brain activity (daemon health check)
brain stats                                    # memory counts overview
brain meta                                     # brain identity
```

### Write

```bash
# project path is auto-detected from CWD; override with --project if needed
brain encode --user <id> --name <name> --agent codex --prompt "user's message" --response "your response"
```

That's the main write command. The brain extracts facts, episodes, interaction metadata, and tags automatically via its local LLM.

For manual overrides (rarely needed):

```bash
brain working --user <id> --agent codex <text>                               # scratchpad (auto-expires)
brain fact --tags t1,t2 --user <id> --agent codex [--project /path] <text>  # store a fact manually
brain episode --tags t1,t2 --user <id> --agent codex [--project /path] <text> # record an episode manually
brain interact --id <id> --name <name> --entity human --outcome success --notes "context"
```

## Protocol

**CRITICAL:** Run **one command when you receive a prompt** and **one command after your response**. Do not run multiple separate brain commands (whoami, attention, recall, etc.) — use the combined commands below.

### START of every conversation (one command only)

**Execute exactly one shell command:**

```bash
brain context [--topics "topic1,topic2,topic3"]
```

- Extract 1–3 topics from the user's message (e.g. "Fix the bug in authentication" → `--topics "authentication,bug"`). If no clear topics, omit `--topics` or use one general term.
- Output includes: whoami, attention, recall and episodes per topic, recent episodes, and traits for the current user.
- If output shows **user: unknown** (no recent human): ask **"Hi! I don't think we've met yet — what should I call you?"**; when they reply, run `brain interact --id user-<slug> --name <Name> --entity human --outcome success --notes "First meeting"` and `brain working --user user-<slug> --agent codex "Active user: user-<slug> (<Name>)"`.
- Known user: greet **"Hi {name}! (If this isn't you, just let me know who you are.)"** and use the context output in your reply.
- If they say they are someone else: run `brain contacts --type human` to find or add them, then `brain working --user <id> --agent codex "Active user: <id> (<name>)"`.

Use the recalled facts, episodes, and traits from the single `brain context` output. Do not run additional brain read commands unless needed (e.g. `brain contacts` for a different user).

### AFTER each response (one command only)

Send the conversation turn to the brain:

```bash
brain encode --user <user-id> --name <Name> --agent codex --prompt "the user's message" --response "your full response"
```

> **Note:** If your response was very long, pass a concise summary in `--response` rather than the full text to avoid bloating the brain.

### Notes

- **Always include `--agent` on brain write commands** (`encode`, `working`, `fact`, `episode`) so each memory records which agent created it.
- **Always include `--user` and `--name`** so memories are associated with the right person.
- **Project context is auto-captured** -- `brain encode` stores the current working directory as the project path automatically. No action needed.
- **Use the context output** -- The single `brain context` run gives you facts, episodes, and traits; use them before acting and in your response.
- The brain file persists across all sessions and projects.
