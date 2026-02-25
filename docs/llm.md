# LLM Integration

See [Prerequisites](../README.md#prerequisites) for when an LLM is required. brAIn uses a connection to any OpenAI-compatible API (Ollama, OpenAI, vLLM, LiteLLM) for three autonomous capabilities:

| Capability | What it does |
|---|---|
| **Memory summarization** | Compresses old episodes into semantic facts during consolidation |
| **Conversation ingestion** | Extracts episodes, facts, and interaction metadata from raw conversation turns |
| **Profile inference** | Writes or updates person schema and behavioral priors for human contacts |

All three are used automatically when you configure an LLM in `~/.brain/config` or via environment variables. The daemon requires an LLM and will fail to start if not configured.

## Environment variables

The CLI and daemon read LLM settings from `~/.brain/config` and environment variables:

| Variable | Default | Description |
|---|---|---|
| `BRAIN_LLM_URL` | `http://localhost:11434` | LLM endpoint |
| `BRAIN_LLM_MODEL` | `qwen3:4b` | Model name |
| `BRAIN_LLM_API_KEY` | *(not set)* | API key (optional, for non-Ollama providers) |

Environment variables override config file values.

## Embeddings

Semantic memory, episodic search, and procedural skill search use **embedding-based similarity**: content is embedded with an embedding model and stored in the `memories` table; queries (e.g. `brain recall`, `brain skill`) use the same model so that related memories are found even when keywords don’t match exactly (e.g. "concurrency" can match "goroutines").

- **Config:** Set `embed_model` in `~/.brain/config` or the `BRAIN_EMBED_MODEL` environment variable. Default (when using Ollama) is `nomic-embed-text`; the daemon ensures this model is available at startup along with the LLM model (see [Daemon](daemon.md)).
- **When embeddings are used:** Recall (semantic lookup), episodic search by topic, and procedure/skill search. If no embedding model is configured, the brain falls back to keyword-style search where supported.
- The daemon and CLI use the same config; the daemon pulls the embed model on startup when using an Ollama endpoint (see [Daemon](daemon.md)).
