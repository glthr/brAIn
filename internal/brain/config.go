package brain

import (
	"log/slog"
	"time"
)

// Option configures a Brain.
type Option func(*config)

type config struct {
	consolidationInterval time.Duration
	decayRate             float64
	salienceFloor         float64
	semanticizationFunc   SemanticizationFunc
	semanticizationAge    time.Duration
	workingMemoryTTL      time.Duration
	inferProfileFunc      InferProfileFunc
	batchEncoderFunc      BatchEncoderFunc
	profileInterval       time.Duration
	logger                *slog.Logger

	// LLM connection (e.g. Ollama)
	llmBaseURL string
	llmModel   string
	llmAPIKey  string

	// Embedding model for semantic vector search (optional).
	// Uses the same llmBaseURL. Set via WithEmbeddingModel.
	embedModel string

	description string

	// Track which functions were explicitly set (to distinguish nil from "not set")
	semanticizationFuncSet bool
	inferProfileFuncSet    bool
	batchEncoderFuncSet    bool

	// Service mode: when true, logs initialization at INFO level (for daemon/service).
	// When false, logs at DEBUG level (for CLI commands).
	serviceMode bool
}

func defaultConfig() *config {
	return &config{
		consolidationInterval: 0,
		decayRate:             0.05,
		salienceFloor:         0.1,
		semanticizationAge:    72 * time.Hour,
		workingMemoryTTL:      30 * time.Minute,
	}
}

// WithConsolidationInterval sets the automatic consolidation period.
// Set to 0 to disable (manual-only via Brain.Consolidate).
func WithConsolidationInterval(d time.Duration) Option {
	return func(c *config) { c.consolidationInterval = d }
}

// WithDecayRate controls how fast salience decreases per consolidation cycle.
func WithDecayRate(r float64) Option {
	return func(c *config) { c.decayRate = r }
}

// WithSalienceFloor sets the threshold below which memories are forgotten.
func WithSalienceFloor(f float64) Option {
	return func(c *config) { c.salienceFloor = f }
}

// WithSemanticizationFunc provides the LLM callback for episodic compression.
// Semanticization (semantic consolidation) is the neuroscience term for the process
// by which episodic detail is compressed into generalizable semantic knowledge
// during systems consolidation.
func WithSemanticizationFunc(fn SemanticizationFunc) Option {
	return func(c *config) {
		c.semanticizationFunc = fn
		c.semanticizationFuncSet = true
	}
}

// WithSemanticizationAge sets the age after which episodic memories are candidates
// for semanticization into semantic knowledge.
func WithSemanticizationAge(d time.Duration) Option {
	return func(c *config) { c.semanticizationAge = d }
}

// WithWorkingMemoryTTL sets how long working memories live before expiring.
func WithWorkingMemoryTTL(d time.Duration) Option {
	return func(c *config) { c.workingMemoryTTL = d }
}

// WithInferProfileFunc provides the LLM callback for automatic user profile
// analysis. The function receives a structured summary of a user's memories and
// interactions, and returns person schema and behavioral priors text to store.
func WithInferProfileFunc(fn InferProfileFunc) Option {
	return func(c *config) {
		c.inferProfileFunc = fn
		c.inferProfileFuncSet = true
	}
}

// WithBatchEncoderFunc provides the LLM callback that processes all pending
// conversation turns in one call. See Brain.Encode and ProcessEncodings.
func WithBatchEncoderFunc(fn BatchEncoderFunc) Option {
	return func(c *config) {
		c.batchEncoderFunc = fn
		c.batchEncoderFuncSet = true
	}
}

// WithProfileInterval sets how often the background profiler runs.
// Set to 0 to disable automatic profiling (manual-only via Brain.AnalyzeProfiles).
func WithProfileInterval(d time.Duration) Option {
	return func(c *config) { c.profileInterval = d }
}

// WithDescription sets a free-form description for this brain.
func WithDescription(desc string) Option {
	return func(c *config) { c.description = desc }
}

// WithOllama configures the brain to use a local Ollama instance for
// autonomous memory consolidation (semanticization) and user trait inference.
// The baseURL is typically "http://localhost:11434" and model is e.g. "qwen3:4b".
// This automatically sets SemanticizationFunc and InferTraitsFunc.
// MANDATORY: Either WithOllama or WithLLM must be provided -- the brain will fail
// to initialize without an LLM connection.
func WithOllama(baseURL, model string) Option {
	return func(c *config) {
		c.llmBaseURL = baseURL
		c.llmModel = model
	}
}

// WithLLM configures the brain to use an Ollama HTTP API endpoint for
// memory consolidation and trait inference. For Ollama, prefer WithOllama
// which sets defaults.
// MANDATORY: Either WithOllama or WithLLM must be provided -- the brain will fail
// to initialize without an LLM connection.
func WithLLM(baseURL, model, apiKey string) Option {
	return func(c *config) {
		c.llmBaseURL = baseURL
		c.llmModel = model
		c.llmAPIKey = apiKey
	}
}

// WithLogger overrides the default file-based logger with a custom slog.Logger.
// By default the brain logs to ~/.brain/brain.log. Use this option to redirect
// log output (e.g. to stderr, a test buffer, or a remote sink).
func WithLogger(l *slog.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithServiceMode enables service-level logging (INFO level for initialization).
// Use this for the daemon/service that runs continuously. CLI commands should
// not use this option, so they remain quiet unless debug logging is enabled.
func WithServiceMode() Option {
	return func(c *config) { c.serviceMode = true }
}

// WithEmbeddingModel enables semantic vector search using the given Ollama embedding model.
// The model is called via the same LLM base URL configured by WithOllama or WithLLM.
// Recommended model: "nomic-embed-text" (fast, 768-dim, excellent quality).
// When set, brain.recall benefits from semantic gap coverage: "concurrency" can match
// a memory tagged "goroutine", "rate limiting" can match "429 errors", etc.
func WithEmbeddingModel(model string) Option {
	return func(c *config) { c.embedModel = model }
}
