# Conversation Ingestion -- How Raw Turns Become Memories

brAIn includes a two-phase ingestion pipeline that turns raw conversation turns into structured memories:

**Phase 1: Fast capture** (synchronous, no LLM). The agent calls `Brain.Encode()` with the user's prompt, the agent's response, and the AI agent's name (e.g. `"cursor"`, `"claude-code"`). This stores the raw conversation as a working memory tagged with `type=conversation`, the user ID, and the originating agent. The write is instant -- no LLM call blocks the agent's response.

```go
id, _ := b.Encode(ctx, "user-alice", "Alice", "cursor", "/home/user/myproject", "How do I test Go?", "Use table-driven tests...")
```

**Phase 2: LLM extraction** (asynchronous, via daemon or manual). `Brain.ProcessEncodings()` finds unprocessed conversation working memories and runs each through the configured `EncoderFunc` callback. The LLM extracts:

- An **episode summary** (stored in episodic memory)
- **Reusable facts** (stored in semantic memory)
- An **interaction record** with outcome and valence for agents (valence not used for humans; trust updated for agents only)
- **Tags** for searchability

After successful extraction, the raw working memory is deleted.

```go
stats, _ := b.ProcessEncodings(ctx)
// stats.Processed=3, stats.Episodes=3, stats.Facts=5, stats.Interactions=3
```

The daemon runs ingestion processing on a timer (default: every 5 minutes), so the agent never has to think about it.
