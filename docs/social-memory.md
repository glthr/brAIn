# Social Memory -- Building a Theory of Mind

The [lifecycle](lifecycle.md) covered how *experiences* graduate from short-term to long-term. Social memory is different -- it tracks *entities*, not events. Every agent and every human the brain encounters gets a persistent profile and an interaction log. For agents, a trust score evolves over time; for humans, no trust score is computed.

## The Interaction Log

Every time the agent interacts with another entity, it records a structured `Interaction`:

| Field | Type | Purpose |
|---|---|---|
| `ContactID` | `string` | Unique identifier of the other entity |
| `ContactName` | `string` | Human-readable name |
| `EntityType` | `EntityType` | `"agent"` or `"human"` |
| `Type` | `string` | Category: `"delegation"`, `"conversation"`, `"code_review"`, etc. |
| `Outcome` | `string` | Result: `"success"`, `"partial"`, `"failure"` |
| `Valence` | `float64` | Quality score -1.0 to 1.0 (agents only; not stored or used for humans) |
| `Notes` | `string` | Free-form context about this specific interaction |
| `Metadata` | `map[string]string` | Arbitrary key-value pairs for structured data |

**A2A scenario:** ResearchBot delegates a web search to SearchBot. SearchBot returns 3 of 5 requested sources. ResearchBot records:

```go
b.Social.RecordInteraction(ctx, brain.Interaction{
    ContactID: "agent-search", ContactName: "SearchBot",
    EntityType: brain.EntityTypeAgent,
    Type: "delegation", Outcome: "partial", Valence: 0.5,
    Notes: "Returned 3/5 sources, missing academic papers",
})
```

**A2H scenario:** ResearchBot explains a concurrency bug to Alice, a senior developer. She resolves it quickly. No valence is recorded for humans:

```go
b.Social.RecordInteraction(ctx, brain.Interaction{
    ContactID: "user-alice", ContactName: "Alice",
    EntityType: brain.EntityTypeHuman,
    Type: "bug_explanation", Outcome: "success",
    Notes: "Understood immediately, fixed in 10 minutes",
})
```

Agent interactions feed into trust scoring; human interactions have no score.

## Trust Scoring (Agents Only) -- How Reputation Emerges

After every `RecordInteraction` with an **agent**, brAIn recalculates that contact's trust score using a recency-weighted formula:

```
trust = SUM(valence × weight) / SUM(weight)

where weight = 1 / (1 + days_since_interaction)
```

The actual SQL (from `social.go`):

```sql
UPDATE contacts SET trust = (
    SELECT COALESCE(
        SUM(valence * (1.0 / (1.0 + (julianday('now') - julianday(created_at))))) /
        NULLIF(SUM(1.0 / (1.0 + (julianday('now') - julianday(created_at)))), 0),
        0.5
    )
    FROM interactions WHERE contact_id = ?
) WHERE id = ?
```

Three important properties:

1. **Recency bias.** An interaction from today has weight `1/(1+0) = 1.0`. One from 30 days ago has weight `1/(1+30) ≈ 0.032`. Recent experience dominates.
2. **No hard cutoff.** Old interactions are never discarded -- they just contribute less. A single bad experience a year ago still has a tiny influence, just as human memory works.
3. **Default-to-neutral.** If there are no interactions (for an agent), `COALESCE` returns `0.5` -- the brain starts with a neutral trust assumption for that agent.

**Worked example:** Suppose an agent has three interactions with SearchBot:

| # | Days ago | Valence | Weight = 1/(1+days) | Weighted valence |
|---|---|---|---|---|
| 1 | 0 (today) | 0.9 | 1.000 | 0.900 |
| 2 | 7 | 0.6 | 0.125 | 0.075 |
| 3 | 30 | -0.2 | 0.032 | -0.006 |

```
trust = (0.900 + 0.075 + (-0.006)) / (1.000 + 0.125 + 0.032)
      = 0.969 / 1.157
      ≈ 0.837
```

The bad experience 30 days ago barely registers. Today's excellent interaction dominates. This mirrors how you trust a colleague more based on this morning's collaboration than a mistake from last month.

## Notes -- Qualitative Context

Not everything fits into a number. `UpdateNotes` attaches free-form text to a contact profile for context that numbers cannot capture:

```go
// A2A: capture operational characteristics
b.Social.UpdateNotes(ctx, "agent-search", "Fast but sometimes incomplete on academic sources. Best for web and news.")

// A2H: capture communication preferences
b.Social.UpdateNotes(ctx, "user-bob", "Junior dev, needs step-by-step walkthroughs. Prefers Go examples over pseudocode.")
```

Notes persist across sessions and are available via `GetContactProfile`. For agents they complement the trust score; for humans they are part of the profile (humans have no score).

## Profile Inference -- LLM-Powered User Understanding

Beyond free-form notes (and agent trust), brAIn builds an evolving **person schema** and **behavioral priors** for each **human** contact. When an LLM is connected (via `WithOllama` or `WithLLM`), the brain's background profiler periodically analyzes each user's interaction history and associated memories, then writes or updates two free-form text fields:

- **Person schema** -- a cognitive model of the individual: communication style, expertise level, preferences, and personality patterns.
- **Behavioral priors** -- predictive expectations: how the user is likely to behave, what they'll ask for next, and how they respond to different types of output.

The profiler runs as a background loop (configurable via `WithProfileInterval` or the daemon's `profile_interval` setting). Each cycle:

1. Finds human contacts with new interactions or memories since their last analysis (`HumansNeedingAnalysis`).
2. Builds a structured summary of the user's profile, recent interactions, and associated memories.
3. Calls the `InferProfileFunc` callback with the summary and the user's current person schema and behavioral priors.
4. Stores the updated person schema and behavioral priors on the contact profile.
5. Stamps `last_analyzed_at` so the user is not re-analyzed until new data arrives.

To avoid profiles being skewed by the first few interactions, **new users** (below a small interaction-count threshold) are also re-queued for analysis periodically (e.g. every 10 minutes) even when there is no new data. The inference prompt instructs the LLM to treat the profile as continuously updated: revise with the full history, avoid overweighting early interactions, and prefer empty schema/priors when data is very sparse.

```go
// Read a user's social schema
socialSchema, _ := b.Social.GetSocialSchema(ctx, "user-alice")
// "Direct communicator. Senior Go developer with deep concurrency expertise.
//  Prefers concise, technical responses. Asks pointed debugging questions..."

// Read behavioral priors
priors, _ := b.Social.GetBehavioralPriors(ctx, "user-alice")
// "Will likely ask follow-up questions about edge cases. Prefers code examples
//  over prose. Expects production-ready patterns, not toy examples."

// Or as part of the full profile
profile, _ := b.Social.GetContactProfile(ctx, "user-alice")
fmt.Println(profile.SocialSchema)
fmt.Println(profile.BehavioralPriors)
```

Both fields are also accessible from the CLI:

```bash
brain traits user-alice    # shows person schema and behavioral priors
brain profile user-alice   # shows full profile including both
```

### When traits are empty (insufficient data)

The profiler runs for every human who has new interactions or memories since their last analysis. It **always** calls the LLM with a summary and then stamps `last_analyzed_at`. However, the inference prompt instructs the LLM to **return empty strings** for `social_schema` and `behavioral_priors` when there is genuinely insufficient evidence — for example, a new user with only one or two interactions and no long-term memories. The LLM must not invent traits with zero supporting data.

So it is **expected** that:

- **New or lightly-used users** may show "No profile yet" or empty traits even after the profiler has run. The profiler did run and call the LLM; the LLM correctly returned nothing to store.
- **Traits appear later** as more interactions and memories accumulate. Each profiler cycle re-checks humans with new data since `last_analyzed_at`, and **new users** (few interactions) are re-queued periodically so the profile can be refined as evidence grows. Once the summary is rich enough, the LLM will return non-empty schema and priors and they will be stored.

To verify behavior: the daemon logs profile runs in `~/.brain/daemon.log` (e.g. `"analyzed user profile"` with `social_schema_updated` / `behavioral_priors_updated`). The `contacts` table in `~/.brain/agent.brain` has `last_analyzed_at` set once the profiler has processed a user; empty `social_schema` and `behavioral_priors` simply mean the last inference had insufficient data to write.

## User Identification -- "Who am I talking to?"

When a session starts, the agent calls `MostRecentHuman` (exposed as `brain whoami` in the CLI) to identify the last known human user. If a name is returned, the agent greets them by name and offers to switch if someone else is at the keyboard. If no humans are known yet (fresh brain), the agent asks for a name and immediately registers the new user with a unique ID like `user-alice`.

This means the brain supports **multiple human users**, each with their own interaction history and person schema profile. A shared workstation, a team, or a classroom can all use the same brain file -- each person is tracked separately.

## A2A vs A2H -- Same Infrastructure, Different Semantics

A key design decision: agents and humans share the same tables and the same API. Trust is computed and stored only for agents; the `entity_type` column is the discriminator.

```
RecordInteraction()
        │
        ▼
┌──────────────────────┐
│   contacts table     │     entity_type = "agent" or "human"
│                      │     trust, social_schema, behavioral_priors, notes, timestamps
│   ┌──────────────┐   │
│   │ agent-search │───│──┐
│   │ user-alice   │───│──│──┐
│   └──────────────┘   │  │  │
└──────────────────────┘  │  │
                          │  │
┌──────────────────────┐  │  │
│ interactions         │  │  │
│                      │  │  │
│   FK: contact_id ────│──┘  │
│   valence, outcome   │     │
│   type, notes        │─────┘
└──────────────────────┘
        │
        ▼
ListAgents() ── filters entity_type = "agent"
ListHumans() ── filters entity_type = "human"
ListAll()    ── returns both
```

This unified design means interaction logging and profile management apply to both; trust scoring applies to agents only. There is no separate "human contact" system to maintain.

## What Social Memory Enables

Social memory creates emergent behaviors that no system prompt can replicate:

**A2A (agent-to-agent):**
- **Selective delegation.** Route tasks to the peer with the highest trust score for that task type. If SearchBot has trust 0.92 and DataBot has 0.61, the agent delegates search tasks to SearchBot.
- **Automatic reputation tracking.** No explicit "rate this agent" step -- trust emerges from accumulated interaction outcomes.
- **Interaction replay before collaboration.** Before delegating, the agent can review `InteractionHistory` to recall what went wrong last time.

**A2H (agent-to-human):**
- **User identification.** The brain recognizes returning users by name and supports multiple humans with separate profiles, person schema, and behavioral priors.
- **Adaptive communication style.** The LLM-inferred person schema and behavioral priors tell the agent how to communicate -- concise and technical for a senior engineer, step-by-step for a junior developer. This adapts automatically as the profiler re-analyzes with new data.
- **Persistent preference learning.** Explicit preferences and behavioral patterns are captured in the person schema, behavioral priors, and notes, evolving as new interactions accumulate.
- **Trust-gated actions (agents).** An agent might delegate to the peer with the highest trust score; for humans, the brain does not compute trust.
