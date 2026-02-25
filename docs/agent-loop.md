# How an Agent Uses the Brain Day-to-Day

A `.brain` file is not passive storage. It is an active participant in every inference cycle. Here is what a typical agent loop looks like when wired to brAIn:

```
  ┌──────────────────────────────────────────────────────────────────────────┐
  │                      AGENT INFERENCE LOOP                                │
  │                                                                          │
  │  1. RECEIVE task / message                                               │
  │       │                                                                  │
  │  2. REMEMBER ── query the brain ───────────────────────────────────────  │
  │       │    Working:   "What am I in the middle of?"                      │
  │       │    Episodic:  "Have I seen this kind of task before?"            │
  │       │    Semantic:  "What facts do I know about this topic?"           │
  │       │    Procedural:"Do I have a strategy that works here?"            │
  │       │    Social:    "Who is asking? Agent or human?"                   │
  │       │              "What do I know about their personality?"           │
  │       │              "Have I worked with them before?"                   │
  │       │                                                                  │
  │  3. THINK ── build prompt / plan using retrieved context                 │
  │       │                                                                  │
  │  4. ACT ── call the LLM, execute tools, produce output                   │
  │       │                                                                  │
  │  5. LEARN ── write back to the brain ───────────────────────────────     │
  │       │    Encode:    store the conversation turn for extraction         │
  │       │    Working:   store scratchpad for this task                     │
  │       │    Episodic:  record what happened and the outcome               │
  │       │    Semantic:  save any new facts discovered                      │
  │       │    Procedural:update success rate of the strategy used           │
  │       │    Social:    log interaction with valence                       │
  │       │              recalculate trust (agents only)                     │
  │       │                                                                  │
  │  6. REPEAT                                                               │
  └──────────────────────────────────────────────────────────────────────────┘
```

## A concrete example

An agent receives the task "debug this goroutine leak." Before calling the LLM, it queries the brain:

```go
// Do I have a procedure for this?
proc, err := b.Procedural.Lookup(ctx, "debug_goroutine_leak")
// proc.Steps = ["run pprof", "identify leaked goroutines", ...]
// proc.SuccessRate = 0.90

// Have I debugged goroutine leaks before?
past, _ := b.Episodic.Search(ctx, brain.EpisodicSearchOpts{
    Tags: []string{"goroutine", "debugging"}, Limit: 3,
})
// past[0].Content = "Used pprof to find goroutine stuck on unbuffered channel"

// What do I know about goroutines?
facts, _ := b.Semantic.Lookup(ctx, "goroutine", 5)
// facts[0].Content = "Goroutines blocked on a nil channel block forever"

// Who is asking? Agent or human?
profile, _ := b.Social.GetContactProfile(ctx, requesterID)
// profile.EntityType = "human" (Trust is not used for humans.)
// profile.SocialSchema = "Senior Go developer, prefers concise technical answers..."
```

The social profile and personality shape how the agent responds:

```go
// Adapt behavior based on social schema (and agent trust when delegating)
if profile.EntityType == "human" {
    if strings.Contains(profile.SocialSchema, "senior") {
        // Concise, technical response
    } else {
        // Detailed explanation with examples
    }
}
```

The agent now has a procedure to follow, past experiences to draw from, relevant facts, and a personality-aware profile for the requester (and trust when the requester is an agent) -- all retrieved in milliseconds from the local `.brain` file.

After finishing, it writes back:

```go
// Record the experience
b.Episodic.Record(ctx, brain.Episode{
    Content: "Debugged goroutine leak: channel was never closed in defer",
    Tags:    []string{"goroutine", "debugging", "success"},
})
b.Procedural.UpdateSuccessRate(ctx, "debug_goroutine_leak", true)

// Encode the full conversation turn (LLM extracts memories later)
b.Encode(ctx, requesterID, requesterName, "cursor", projectPath, userPrompt, agentResponse)
```

The brain is now slightly smarter than it was before. Next time, this experience will surface -- and if the requester was an agent, its trust score will be updated when the daemon processes the ingested turn.

## Does the brain give the agent a personality?

Yes. Not a scripted personality, but an emergent one.

Think of it as nature versus nurture. The LLM is nature -- the base capabilities, the language, the reasoning patterns. The `.brain` file is nurture -- the lived experience that shapes behavior.

Two agents running the same LLM but carrying different `.brain` files will behave differently:

| Agent A (`.brain` from a code review team) | Agent B (`.brain` from a support team) |
|---|---|
| Knows `code_review` procedure at 92% success | Knows `troubleshoot_user_issue` at 88% success |
| Trusts CodeGenBot (score 0.9) | Person schema, behavioral priors (no trust) |
| Semantic memory full of API patterns | Semantic memory full of product FAQ |
| Prefers structured, terse responses (learned) | Prefers empathetic, step-by-step responses (learned) |
| Social graph: 12 agents, 3 humans | Social graph: 2 agents, 40 humans |

This is personality. Not because someone wrote "you are a friendly assistant" in a system prompt, but because the agent's accumulated experiences -- its successes, its failures, who it trusts, what it knows -- naturally bias how it approaches new situations.

You can even **transplant personality**: copy Agent A's `.brain` file to a new machine running a different LLM, and the new agent inherits Agent A's knowledge, strategies, and social relationships. The entire social graph travels with the `.brain` file -- every contact profile, every agent trust score, every person schema, every behavioral prior, every interaction log entry.
