# From Short-Term to Long-Term -- How Memories Graduate

The most important mechanism in brAIn is how a fleeting piece of context becomes a permanent memory. This mirrors the human brain, where not everything you perceive makes it into long-term storage -- only what you pay attention to, rehearse, or find meaningful.

Here is the complete lifecycle:

```
 WORKING MEMORY          created by the agent during a task
     │                   (default TTL: 30 minutes)
     │
     ├── accessed < 3x ──> TTL expires ──> DELETED
     │                     (forgotten, like a phone number
     │                      you heard but never dialed)
     │
     └── accessed 3+ x ──> CONSOLIDATION promotes it
                           │
                           ▼
                    EPISODIC MEMORY
                      (long-term, no expiry)
                           │
                           │  salience decays if never
                           │  accessed again (-5% per cycle)
                           │
                           ├── salience drops below floor ──> FORGOTTEN
                           │   (forgotten, like a mundane Tuesday
                           │    you can no longer recall)
                           │
                           ├── stays relevant (accessed) ──> salience
                           │   holds or grows
                           │
                           └── ages past semanticizationAge ──> SEMANTICIZATION
                               (LLM compresses episodes into facts)
                               │
                               ▼
                        SEMANTIC MEMORY
                          (permanent knowledge)
```

## Step by step

**1. The agent stores working context.** During a task, the agent writes scratchpad entries:

```go
b.Working.Store(ctx, "user is asking about goroutine leaks", nil)
```

This creates a row in the `memories` table with `memory_type = 'working'` and an `expires_at` timestamp 30 minutes in the future.

**2. Access count tracks rehearsal.** Every time the agent reads that memory back (via `Working.Get`), the `retrievals` column increments by 1 and `last_accessed` is updated. This is the equivalent of rehearsal -- the more you repeat something, the more likely it sticks.

**3. The consolidation engine runs.** Either on a background timer or via a manual `b.Consolidate(ctx)` call, the sleep cycle begins. Phase 1 deletes any working memories whose TTL has expired. Then phase 3 checks:

```sql
UPDATE memories SET
    memory_type = 'episodic',
    expires_at = NULL,
    salience = MIN(salience + 0.2, 1.0)
WHERE memory_type = 'working'
  AND retrievals >= 3
  AND (expires_at IS NULL OR expires_at > datetime('now'))
```

If a working memory was accessed 3 or more times *and* hasn't expired yet, it is promoted: its type changes to `episodic`, its expiry is removed (it now lives forever), and its salience gets a +0.2 boost. The row stays in the same table, with the same ID.

**4. The episodic memory lives, decays, or gets compressed.** Once promoted, the memory follows the same lifecycle as any episodic memory:

- Each consolidation cycle applies the Ebbinghaus decay: `salience *= (1 - decayRate)` for memories not accessed in the last 24 hours. With the default rate of 0.05, an untouched memory loses about 5% of its salience each cycle.
- If the agent keeps accessing it (via `Episodic.Recall`, `Semantic.Lookup`, etc.), `last_accessed` resets, and the decay does not fire for that cycle. Actively-used memories survive.
- If salience drops to the floor (default: 0.1), the memory is pruned -- permanently deleted.
- If the memory survives long enough to pass `semanticizationAge` (default: 72 hours), and a `SemanticizationFunc` callback is configured, it becomes a candidate for compression. The consolidation engine groups old episodes by day, passes each group to the LLM callback, and replaces the original episodes with a single semantic memory.

**5. Semantic memories are the most stable.** They are still subject to decay and pruning, but because they tend to be accessed during keyword lookups (which call `Potentiate` internally), actively-relevant knowledge naturally maintains its salience. Rarely-used facts gradually fade, just as you forget trivia you never use.

## What this means in practice

An agent running with a 10-minute consolidation interval (the daemon default) will naturally:

- Forget the scratchpad from tasks it only touched briefly (working memory expires).
- Retain context it kept coming back to during a long task (promoted to episodic).
- Gradually distill old experiences into compact knowledge (episodic compressed to semantic).
- Lose irrelevant facts over weeks if they are never queried (decay + prune).
- Keep core knowledge alive indefinitely if it remains useful (access resets decay).

No manual curation is needed. The agent's own usage patterns determine what it remembers.

## Consolidation -- "Sleeping on It"

The human brain consolidates memories during sleep: it replays the day, strengthens important connections, prunes noise, and compresses episodic details into general knowledge. brAIn does the same thing.

Consolidation runs as a background goroutine on a configurable interval (or can be triggered manually). Each cycle executes five phases:

```
 Phase 1: EXPIRE         Delete working memories past their TTL.
    |
 Phase 2: DECAY          Apply Ebbinghaus forgetting curve.
    |                     salience *= (1 - decayRate) for each
    |                     memory not accessed in the last 24 hours.
    |
 Phase 3: TRANSFER        Working memories accessed 3+ times become
    |                     episodic memories (short-term -> long-term).
    |
 Phase 4: FORGET          Delete memories whose salience has decayed
    |                     below the floor (default: 0.1).
    |
 Phase 5: SEMANTICIZE   If a SemanticizationFunc callback is provided,
                          group old episodes by day and invoke the
                          callback to compress them into semantic
                          memories. Original episodes are deleted.
```

After each cycle, stats are written to the `consolidation_log` table so you can observe the brain's housekeeping over time.
