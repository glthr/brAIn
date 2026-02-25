# How It Maps to the Human Brain

The human brain does not have a single "memory." It has specialized subsystems, each optimized for a different kind of information. brAIn mirrors this:

```
┌─────────────────────────────────────────────────────────────────┐
│                           brAIn                                 │
│                                                                 │
│  ┌────────────────┐  Prefrontal Cortex                          │
│  │ Working Memory │  What am I doing right now?                 │
│  │                │  Auto-expires. Scratchpad for active tasks. │
│  └────────┬───────┘                                             │
│           │                                                     │
│  ┌────────┴───────┐  Hippocampus                                │
│  │Episodic Memory │  What happened?                             │
│  │                │  Timestamped events and experiences.        │
│  └────────┬───────┘                                             │
│           │                                                     │
│  ┌────────┴───────┐  Temporal Lobe                              │
│  │Semantic Memory │  What do I know?                            │
│  │                │  Facts, concepts, general knowledge.        │
│  └────────┬───────┘                                             │
│           │                                                     │
│  ┌────────┴───────┐  Cerebellum / Basal Ganglia                 │
│  │Procedural Mem. │  How do I do things?                        │
│  │                │  Learned action sequences and strategies.   │
│  └────────┬───────┘                                             │
│           │                                                     │
│  ┌────────┴───────┐  Temporo-Parietal Junction                  │
│  │ Social Memory  │  Who have I worked with?                    │
│  │                │  Agent AND human profiles history,          │
│  │                │  person schema, behavioral priors.          │
│  └────────────────┘                                             │
└─────────────────────────────────────────────────────────────────┘
```

## Working Memory -- "What am I doing right now?"

Like the prefrontal cortex holding a phone number you just heard. Working memories are ephemeral: they auto-expire after a configurable TTL (default: 30 minutes). They are the scratchpad for an agent's current task.

Working memories can be associated with a specific user via `StoreForUser`, which tags them with a user ID. This is how conversation ingestion works -- raw turns are stored as working memories tagged to the user who sent them.

If a working memory gets accessed frequently (3+ times), the consolidation engine recognizes it as important and **promotes** it to episodic memory, just as rehearsal moves information from short-term to long-term storage in the human brain.

## Episodic Memory -- "What happened?"

Like the hippocampus replaying your day. Each episode is a timestamped record of something the agent experienced: a conversation, a task outcome, an error. Episodes carry tags and metadata so the agent can search for past experiences by time range, topic, or importance.

Over time, old episodes are candidates for **summarization** -- the consolidation engine can invoke an LLM to compress a cluster of old episodes into a single semantic fact, the same way your brain consolidates a week of commuting into the general knowledge "the 8 AM train is usually crowded."

## Semantic Memory -- "What do I know?"

Like your temporal lobe storing the fact that Paris is in France. These are context-free facts and concepts. Semantic memories are searchable by keyword using semantic similarity search (when embeddings are configured) and ranked by importance -- the agent can ask "find facts about concurrency" and get relevant results even if the stored memory uses different terminology.

## Procedural Memory -- "How do I do things?"

Like the cerebellum remembering how to ride a bike. A procedure is a named sequence of steps with a tracked success rate and use count. Each time the agent uses a procedure and reports the outcome, the success rate is updated via exponential moving average -- the agent naturally learns which strategies work and which do not.

### Emerging skills (skills that emerge from interaction)

**Could the brain learn skills from interaction?** Yes. An **emerging skill** can come from (1) the user teaching steps (e.g. "Open in LibreOffice, then File → Export as PDF") or (2) the agent or user discovering what works by trying different approaches. The brain identifies **(a) a goal**, **(b) steps or procedure**, and **(c) a quantification of success**, and stores **only successful** procedures for a given goal.

**Do we need a new table?** No. The existing **procedural memory** (the same store used by `brain procedure` and `brain procedures`) is the right place. An "emerging skill" is just a procedure **extracted from conversation or consolidation** instead of being added explicitly via the CLI. Same schema: name, description, steps, success rate, use count.

| Source | How it gets in | Example |
|--------|----------------|---------|
| **Explicit** | User or agent runs `brain procedure --name X --steps "a,b,c"` | Stored during or after a task |
| **Emerging** | Encoder or consolidation LLM detects a successful procedure (goal + steps + success) from conversation or from discovered approaches | User taught "convert to PDF via LibreOffice Export" or agent found that running X then Y fixed the issue → stored as procedure |

The **consolidation** system prompt includes a section ["Recognizing emerging skills"](../internal/llm/prompts/consolidate.md) so the LLM can identify patterns that reveal an emerging skill when compressing episodic memories.

**Example behaviors**

- **User teaches a how-to.** You say: "To get a PDF from this doc I always open it in LibreOffice, then File → Export as PDF." The conversation is encoded; during consolidation the LLM emits an `EMERGING_SKILL:` line (goal + steps). The pipeline stores it in procedural memory. Later, when you ask "how do I turn this into a PDF?", the agent runs `brain skill "convert to PDF"`, gets back the LibreOffice steps, and follows them instead of guessing.
- **Agent discovers what works.** You and the agent debug a goroutine leak: you run `go tool pprof`, identify a goroutine stuck on an unbuffered channel, add a close in a defer, and the leak is fixed. That episode is consolidated; the LLM outputs a semantic fact plus `EMERGING_SKILL: debug goroutine leak (run pprof; identify blocked goroutines; fix channel lifecycle)`. Next time you say "I think there's a goroutine leak", the agent can run `brain skill "goroutine leak"`, retrieve the procedure, and apply the same strategy.
- **Repeated workflow becomes a skill.** You often say "when tests fail I run `go test -v ./...` first to see which package breaks." Over several episodes the pattern is clear; consolidation summarizes it and may emit an emerging skill for "diagnose failing tests". The agent then has a reusable procedure for that goal alongside any explicitly stored procedures.

**Do we need a new table or new commands?** No new table — emerging skills live in the same **procedural memory** (same `memories` rows with `memory_type = 'procedural'`). What is needed so the agent can search and apply them:

| Piece | Status |
|-------|--------|
| **Storage** | Same procedural store; no new table. |
| **Persisting from consolidation** | Implemented: consolidation parses `EMERGING_SKILL:` lines from the LLM output and stores them in procedural memory (daemon uses this). |
| **Search and apply** | **`brain skill <query>`** — full-text search over procedures (goal/name and description), returns matching procedures with steps so the agent can apply one. Use this when the user asks for a task that might match an existing or emerging skill. |
| **List all** | **`brain procedures`** — already exists; lists all procedures (explicit + emerging). |

So: no new table; the pipeline persists `EMERGING_SKILL:` into procedural memory, and the agent uses **`brain skill <goal/intent>`** to search and apply a preexisting emerging (or explicit) skill.

## Social Memory -- "Who have I worked with?"

Like the temporo-parietal junction modeling other people's intentions. An agent does not operate in isolation -- it interacts with **other agents** (A2A) and with **humans** (A2H). Social memory tracks both, including a persistent, evolving person schema profile for each contact.

Every contact is stored with an `entity_type` that is either `"agent"` or `"human"`. This lets the brain distinguish between:

| Dimension | Agent (A2A) | Human (A2H) |
|---|---|---|
| **Typical interactions** | delegation, collaboration, query | conversation, code review, feedback |
| **Trust** | recency-weighted from interaction valence (agents only) | not used (humans have no trust score) |
| **Why it matters** | Decide which peer to delegate to | Adapt tone, detail level, patience |

The neuroscience parallel runs deep. The human TPJ enables **theory of mind** -- the ability to model other people's intentions and predict their behavior. brAIn's social memory does the same for **agents**: it builds a running model of each agent the brain interacts with, tracking reliability and trustworthiness over time. **Humans** are not assigned a trust score; the brain tracks their identity, interaction history, person schema, and behavioral priors only.

For **agents**, each interaction is logged with a valence score (-1.0 to 1.0) and brAIn computes a **trust score** using a recency-weighted average. **Humans** have no valence or score; the brain tracks outcome and notes only.

Each contact profile contains:

| Field | Type | A2A example | A2H example |
|---|---|---|---|
| `id` | `string` | `"agent-42"` | `"user-alice"` |
| `entity_type` | `EntityType` | `"agent"` | `"human"` |
| `name` | `string` | `"SearchBot"` | `"Alice"` |
| `trust` | `float64` | `0.92` (agents) | not used (0 for humans) |
| `notes` | `string` | `"Fast but sometimes incomplete"` | `"Senior dev, prefers concise answers"` |
| `social_schema` | `string` | `""` | `"Direct communicator, senior Go dev..."` |
| `behavioral_priors` | `string` | `""` | `"Prefers concise responses, asks pointed questions..."` |
| `interaction_count` | `int` | `47` | `12` |
| `avg_valence` | `float64` | `0.85` | not used (0 for humans) |
| `last_interaction` | `time.Time` | `2026-02-17 09:30:00` | `2026-02-16 14:00:00` |

You can list contacts by type, record interactions, and retrieve history:

```go
// List contacts by type
agents, _ := b.Social.ListAgents(ctx)  // only entity_type = "agent"
humans, _ := b.Social.ListHumans(ctx)  // only entity_type = "human"
all,    _ := b.Social.ListAll(ctx)     // both

// Record an A2A interaction
b.Social.RecordInteraction(ctx, brain.Interaction{
    ContactID: "agent-42", ContactName: "SearchBot",
    EntityType: brain.EntityTypeAgent,
    Type: "delegation", Outcome: "success", Valence: 0.9,
})

// Record an A2H interaction
b.Social.RecordInteraction(ctx, brain.Interaction{
    ContactID: "user-alice", ContactName: "Alice",
    EntityType: brain.EntityTypeHuman,
    Type: "code_review", Outcome: "success", Valence: 0.8,
})
```

Free-form notes can be attached to each profile (e.g., "prefers structured prompts", "junior dev -- needs extra explanation"). See [Social Memory](social-memory.md) for trust scoring (agents), interaction logging, and profile inference in detail.
