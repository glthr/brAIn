# brAIn vs. Static Instruction Files

AI editors give you a way to persist guidance across conversations: `CLAUDE.md` for Claude Code, `AGENTS.md` for Codex, `.cursorrules` or `.mdc` files for Cursor. These files are useful and widely adopted. brAIn is not a replacement for them: it is a solution to a different problem they cannot address.

---

## The appeal of static instruction files

Static instruction files are simple and effective for what they do. You write your preferences once -- preferred language, code style, project conventions, forbidden patterns -- and every conversation with the agent starts with those instructions already loaded. No setup per session. No configuration drift. The agent behaves consistently because the rules are written down.

They work especially well for immutable, project-wide constraints: "always use TypeScript strict mode", "never use `any`", "run `make test` before committing". These rules don't change based on who's working or what happened last week, so a static file is the right tool.

---

## What they can't do

Static instruction files are text. They don't execute, learn, or adapt. That constraint has consequences:

**They never update themselves.** If you discover a new debugging approach that works well, you have to remember to open `CLAUDE.md` and write it down. The agent won't. Next session, the approach is lost unless you manually captured it.

**They require manual curation.** Every useful fact, preference, and learned pattern must be authored by you. The agent accumulates experience but has no mechanism to persist it. You're maintaining the agent's "memory" by hand.

**They know nothing about the user.** A `CLAUDE.md` file applies identically to every person who opens the project. A senior engineer and a junior intern get the same behavior, the same verbosity, the same explanations. There's no user modeling.

**Each session still starts from zero.** Instruction files are context, not memory. They tell the agent what it should know, but the agent has no recollection of what actually happened in past sessions. It can't say "last time we tried X and it failed" or "you said you prefer Y" unless you wrote it down.

**They grow without bound.** The more you rely on instruction files, the larger they get. Every new rule, every project detail, every learned lesson consumes more context window on every turn. The file gets unwieldy; the relevant parts get buried; you start trimming things that matter.

**They're siloed per agent and per project.** A `CLAUDE.md` in your project root doesn't share knowledge with Codex's `AGENTS.md` or Cursor's `.mdc` rules. Each agent starts its own blank slate. Work done in one tool doesn't carry over to another.

---

## What brAIn does differently

brAIn addresses each of these limitations directly.

**Memory is dynamic, not static.** When an agent encodes a conversation turn (`brain encode`), the brain's LLM extracts facts, episodes, and procedural patterns automatically. Nothing requires manual authoring. The brain grows as the agent works.

**No manual curation.** The consolidation engine ("sleep cycle") runs in the background, extracting what's worth keeping, merging duplicates, and decaying what's no longer relevant. The agent's memory self-organizes. You don't have to.

**User modeling is built in.** brAIn maintains a contact record and behavioral profile for each person the agent has interacted with. Traits (verbosity preference, expertise level, communication style) are inferred from interaction history and made available at the start of every conversation via `brain context`. The same agent behaves differently with different users -- not because you configured it, but because it learned.

**Cross-session continuity.** At the start of every conversation, the agent runs `brain context` and receives: who the user is, what's currently active (working memory), relevant recalled facts and episodes for the current topic, recent history, and behavioral priors. The conversation resumes with context, not from scratch.

**Built-in forgetting curve.** brAIn implements Ebbinghaus-style memory decay. Memories that are never accessed fade over time; memories that are recalled repeatedly are reinforced. The brain stays relevant without growing indefinitely. Context cost stays bounded.

**Agent-agnostic, project-agnostic.** One `.brain` file at `~/.brain/agent.brain` is shared across Claude Code, Codex, Cursor, Gemini CLI, and any other tool that uses the brain protocol. Knowledge learned in one editor is immediately available in another. Work done in one project is available in any project (for cross-project facts) or scoped to the right project automatically.

---

## Comparison table

| Dimension | Static files (CLAUDE.md, AGENTS.md, .cursorrules) | brAIn |
|---|---|---|
| **Nature** | Authored text, read on load | Structured database, queried per turn |
| **Updates** | Manual -- you edit the file | Automatic -- the brain extracts and stores |
| **Learning** | None -- the agent can't write back | Continuous -- every turn encodes new memory |
| **User modeling** | None -- same behavior for everyone | Per-user profiles, traits, behavioral priors |
| **Cross-session memory** | None -- each session re-reads the same file | Full -- episodes, facts, working memory persist |
| **Multi-agent sharing** | None -- each agent reads its own file | Full -- one `.brain` file shared across all agents |
| **Size over time** | Grows with manual additions; never shrinks | Self-regulating via Ebbinghaus decay |
| **Setup cost** | Low -- write a file, done | Low -- `make install` wires up all editors at once |

---

## They're complementary, not competing

Here's the key insight: brAIn *uses* static instruction files to deliver its protocol to each agent. The `CLAUDE.md` that brAIn installs into `~/.claude/CLAUDE.md` is itself a static instruction file -- it tells Claude Code to run `brain context` at the start of each turn and `brain encode` at the end. The same pattern applies to `AGENTS.md` for Codex and `.mdc` for Cursor.

The static file answers: *how should the agent interact with the brain?*
The brain answers: *what does the agent know, and who is it talking to?*

Neither achieves much without the other. The static file is the delivery mechanism; the brain is the persistent, evolving state. Together they give the agent a stable protocol *and* real memory -- something neither file alone can provide.

---

## When static files are still the right tool

Static instruction files remain the right choice for anything that genuinely shouldn't change:

- **Project conventions** -- language, framework, module structure, naming rules
- **Style guides** -- formatting, comment style, preferred idioms
- **Hard constraints** -- forbidden libraries, required linting passes, commit message format
- **Onboarding context** -- what the project does, how to run it, where the tests live

These are facts about the *project*, not about the user or the agent's accumulated experience. They're stable, project-scoped, and appropriate for every person who opens the repo. A static file is exactly the right representation.

brAIn is for everything that should *learn and adapt*: how the user prefers to communicate, what debugging approaches worked on this codebase, which decisions were made and why, what the agent tried last week. That's memory -- and memory belongs in a brain, not a text file.
