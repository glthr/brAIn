package brain

import (
	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/memory"
	"github.com/glthr/brAIn/internal/model"
)

// ---- Type aliases re-exported from internal/model ----

type (
	Memory                = model.Memory
	MemoryType            = model.MemoryType
	EntityType            = model.EntityType
	Episode               = model.Episode
	Fact                  = model.Fact
	Procedure             = model.Procedure
	Interaction           = model.Interaction
	ContactProfile        = model.ContactProfile
	EpisodicArchive       = model.EpisodicArchive
	EpisodicSearchOpts    = model.EpisodicSearchOpts
	SemanticSearchOpts    = model.SemanticSearchOpts
	ConsolidationStats    = model.ConsolidationStats
	SemanticizationFunc   = model.SemanticizationFunc
	BrainMeta             = model.BrainMeta
	ProfileInference      = model.ProfileInference
	InferProfileFunc      = model.InferProfileFunc
	BatchEncoderFunc      = model.BatchEncoderFunc
	EncodingTurn          = model.EncodingTurn
	EncodingResult        = model.EncodingResult
	ProfileAnalysis       = model.ProfileAnalysis
	EncodingStats         = model.EncodingStats
	ConsolidationLogEntry = model.ConsolidationLogEntry
)

// Engram is the physical trace of a stored memory — an alias for Memory.
// The term comes from neuroscience: an engram is the neural substrate of a
// specific memory, the pattern of synaptic changes that encodes an experience.
type Engram = Memory

// Re-exported constants.
const (
	MemoryTypeWorking    = model.MemoryTypeWorking
	MemoryTypeEpisodic   = model.MemoryTypeEpisodic
	MemoryTypeSemantic   = model.MemoryTypeSemantic
	MemoryTypeProcedural = model.MemoryTypeProcedural
	MemoryTypeGoal       = model.MemoryTypeGoal

	EntityTypeAgent = model.EntityTypeAgent
	EntityTypeHuman = model.EntityTypeHuman

	MetaKeySchemaVersion = model.MetaKeySchemaVersion
	MetaKeyCreatedAt     = model.MetaKeyCreatedAt
	MetaKeyDescription   = model.MetaKeyDescription
	MetaKeyLastBackupAt  = model.MetaKeyLastBackupAt
)

// Re-exported sentinel errors.
var (
	ErrNotFound = model.ErrNotFound
	ErrExpired  = model.ErrExpired
)

// ---- Subsystem type aliases ----

type (
	WorkingMemory    = memory.WorkingMemory
	EpisodicMemory   = memory.EpisodicMemory
	SemanticMemory   = memory.SemanticMemory
	ProceduralMemory = memory.ProceduralMemory
	SocialMemory     = memory.SocialMemory
)

// ---- LLM client re-exports ----

type LLMClient = llm.Client

// NewLLMClient creates a new Ollama LLM client.
// baseURL is typically "http://localhost:11434".
var NewLLMClient = llm.NewClient

// NewSemanticizationFunc returns a SemanticizationFunc backed by the given LLM client.
var NewSemanticizationFunc = llm.NewSemanticizationFunc

// NewInferProfileFunc returns an InferProfileFunc backed by the given LLM client.
var NewInferProfileFunc = llm.NewInferProfileFunc

// EmbedFunc is a function that converts text into a float32 embedding vector.
// Used by SemanticMemory, EpisodicMemory, and ProceduralMemory for semantic similarity search.
type EmbedFunc = llm.EmbedFunc

// NewEmbedFunc returns an EmbedFunc backed by the given LLM client.
// The client's Model field should point to a dedicated embedding model (e.g. "nomic-embed-text").
var NewEmbedFunc = llm.NewEmbedFunc
