package llm

import _ "embed"

//go:embed prompts/consolidate.md
var consolidateSystemPrompt string

//go:embed prompts/traits.md
var inferPersonalitySystemPrompt string

//go:embed prompts/ingest.md
var ingestSystemPrompt string

//go:embed prompts/ingest_batch.md
var ingestBatchSystemPrompt string
