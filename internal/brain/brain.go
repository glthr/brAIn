// Package brain provides a neuroscience-inspired memory system for AI agents.
//
// All data is stored in a single SQLite database. The brain exposes five
// memory subsystems (Working, Episodic, Semantic, Procedural, Social) and
// a consolidation engine that forgets, transfers, and abstracts memories over time.
package brain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gosimple/slug"
	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/glthr/brAIn/internal/consolidation"
	"github.com/glthr/brAIn/internal/llm"
	"github.com/glthr/brAIn/internal/memory"
	"github.com/glthr/brAIn/internal/model"
	"github.com/glthr/brAIn/internal/profiler"
	"github.com/glthr/brAIn/internal/store"
)

type Brain struct {
	Working    *WorkingMemory
	Episodic   *EpisodicMemory
	Semantic   *SemanticMemory
	Procedural *ProceduralMemory
	Social     *SocialMemory

	cfg          *config
	store        *store.Store
	db           *sql.DB
	logger       *slog.Logger
	logFile      io.Closer
	consolidator *consolidation.Consolidator
	profiler     *profiler.Profiler

	cancelConsolidation context.CancelFunc
	cancelProfiler      context.CancelFunc
	wg                  sync.WaitGroup
	closeOnce           sync.Once
}

// BrainExt is the canonical file extension; the file is standard SQLite.
const BrainExt = ".brain"

const SchemaVersion = "1"

// New opens or creates a brain. Path gets .brain appended if needed; use ":memory:" for in-memory.
func New(dbPath string, opts ...Option) (*Brain, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	if dbPath != ":memory:" && !strings.HasSuffix(dbPath, BrainExt) {
		ext := filepath.Ext(dbPath)
		if ext != "" {
			dbPath = strings.TrimSuffix(dbPath, ext)
		}
		dbPath += BrainExt
	}

	if dbPath != ":memory:" {
		if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("brain: create directory %s: %w", dir, err)
			}
		}
	}

	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=30000"
	if dbPath == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared&_foreign_keys=on"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("brain: open db: %w", err)
	}
	// Single connection avoids pool issues (stale -shm, lock contention) and keeps one file = one brain.
	db.SetMaxOpenConns(1)

	if err := store.InitSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("brain: init schema: %w", err)
	}

	s := store.New(db)

	var logFile io.Closer
	logger := cfg.logger
	if logger == nil {
		if cfg.serviceMode {
			// Daemon: logs to stdout so launchd/systemd control destination (e.g. ~/.brain/daemon.log).
			logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		} else if dbPath == ":memory:" {
			logger = nopLogger()
		} else {
			logFile, logger = openLogFile()
		}
	}

	// LLM is required for encode, process-encodings, consolidate, analyze-profiles. Defaults set unless overridden.
	var lc *llm.Client
	if cfg.llmBaseURL != "" && cfg.llmModel != "" {
		lc = llm.NewClient(cfg.llmBaseURL, cfg.llmModel)
		lc.APIKey = cfg.llmAPIKey

		logLLMInit := logger.Debug
		if cfg.serviceMode {
			logLLMInit = logger.Info
		}

		if !cfg.semanticizationFuncSet {
		} else {
			logLLMInit("LLM semanticization enabled (custom)", "url", cfg.llmBaseURL, "model", cfg.llmModel)
		}
		if !cfg.inferProfileFuncSet {
			cfg.inferProfileFunc = llm.NewInferProfileFunc(lc, logger.With("subsystem", "profiler"))
			logLLMInit("LLM profile inference enabled", "url", cfg.llmBaseURL, "model", cfg.llmModel)
		}
		if !cfg.batchEncoderFuncSet {
			cfg.batchEncoderFunc = llm.NewBatchEncoderFunc(lc)
			logLLMInit("LLM conversation encoding enabled (batch)", "url", cfg.llmBaseURL, "model", cfg.llmModel)
		}
	}

	b := &Brain{
		cfg:     cfg,
		store:   s,
		db:      db,
		logger:  logger,
		logFile: logFile,
	}

	b.Working = memory.NewWorkingMemory(s, cfg.workingMemoryTTL, logger.With("subsystem", "working"))
	b.Episodic = memory.NewEpisodicMemory(s, logger.With("subsystem", "episodic"))
	b.Semantic = memory.NewSemanticMemory(s, logger.With("subsystem", "semantic"))
	b.Procedural = memory.NewProceduralMemory(s, logger.With("subsystem", "procedural"))
	b.Social = memory.NewSocialMemory(s, logger.With("subsystem", "social"))

	var embedFunc llm.EmbedFunc
	if cfg.embedModel != "" && cfg.llmBaseURL != "" {
		ec := llm.NewClient(cfg.llmBaseURL, cfg.embedModel)
		ec.APIKey = cfg.llmAPIKey
		embedFunc = ec.Embed
		b.Semantic.EmbedFunc = embedFunc
		b.Episodic.EmbedFunc = embedFunc
		b.Procedural.EmbedFunc = embedFunc
		logLLMInit := logger.Debug
		if cfg.serviceMode {
			logLLMInit = logger.Info
		}
		logLLMInit("embedding enabled", "model", cfg.embedModel, "url", cfg.llmBaseURL)
	}

	// Default semanticization stores EMERGING_SKILL: lines from consolidation into procedural memory.
	if lc != nil && !cfg.semanticizationFuncSet {
		cfg.semanticizationFunc = llm.NewSemanticizationFunc(lc, emergedSkillAdapter{b})
		if cfg.serviceMode {
			logger.Info("LLM semanticization enabled (with emerging skills)", "url", cfg.llmBaseURL, "model", cfg.llmModel)
		}
	}

	b.consolidator = &consolidation.Consolidator{
		Store:               s,
		DB:                  db,
		Social:              b.Social,
		DecayRate:           cfg.decayRate,
		SalienceFloor:       cfg.salienceFloor,
		SemanticizationFunc: cfg.semanticizationFunc,
		SemanticizationAge:  cfg.semanticizationAge,
		EmbedFunc:           embedFunc,
		Logger:              logger.With("subsystem", "consolidation"),
	}

	if err := b.seedMeta(context.Background(), cfg); err != nil {
		db.Close()
		return nil, fmt.Errorf("brain: seed meta: %w", err)
	}

	b.profiler = &profiler.Profiler{
		Store:  s,
		Social: b.Social,
		Infer:  cfg.inferProfileFunc,
		Logger: logger.With("subsystem", "profiler"),
	}

	if cfg.consolidationInterval > 0 {
		b.startConsolidationLoop()
	}

	if cfg.profileInterval > 0 && cfg.inferProfileFunc != nil {
		b.startProfilerLoop()
	}

	return b, nil
}

// CheckModel verifies the LLM is reachable and the model exists. Call at daemon startup to fail fast.
func (b *Brain) CheckModel(ctx context.Context) error {
	lc := llm.NewClient(b.cfg.llmBaseURL, b.cfg.llmModel)
	lc.APIKey = b.cfg.llmAPIKey
	_, err := lc.Chat(ctx, []llm.Message{{Role: "user", Content: "Hi"}}, 0)
	if err != nil {
		return fmt.Errorf("model %q: %w", b.cfg.llmModel, err)
	}
	return nil
}

// Close stops background loops, runs a PASSIVE WAL checkpoint (non-blocking), then closes log and DB.
// Use Compact() to remove -shm/-wal sidecars (requires exclusive access).
// Idempotent: safe to call multiple times; only the first call runs checkpoint and close.
func (b *Brain) Close() error {
	var closeErr error
	b.closeOnce.Do(func() {
		if b.cancelConsolidation != nil {
			b.cancelConsolidation()
		}
		if b.cancelProfiler != nil {
			b.cancelProfiler()
		}
		b.wg.Wait()

		if b.db != nil {
			if _, err := b.db.ExecContext(context.Background(), `PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
				b.logger.Warn("wal checkpoint failed", "error", err)
			}
			closeErr = b.db.Close()
		}
		if b.logFile != nil {
			_ = b.logFile.Close()
		}
	})
	return closeErr
}

// Compact checkpoints WAL and removes -shm/-wal. Requires exclusive access (fails if daemon has file open).
func (b *Brain) Compact() error {
	if _, err := b.db.ExecContext(context.Background(), `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("brain: wal checkpoint: %w", err)
	}
	return nil
}

func (b *Brain) seedMeta(ctx context.Context, cfg *config) error {
	existing, err := b.store.GetMeta(ctx, model.MetaKeySchemaVersion)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(store.SQLiteTimeFmt)

	if existing == "" {
		if err := b.store.SetMeta(ctx, model.MetaKeySchemaVersion, SchemaVersion); err != nil {
			return err
		}
		if err := b.store.SetMeta(ctx, model.MetaKeyCreatedAt, now); err != nil {
			return err
		}
	}

	if cfg.description != "" {
		if err := b.store.SetMeta(ctx, model.MetaKeyDescription, cfg.description); err != nil {
			return err
		}
	}
	return nil
}

func (b *Brain) GetMemory(ctx context.Context, id int64) (*Memory, error) {
	m, err := b.store.GetMemory(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := b.store.Potentiate(ctx, id); err != nil {
		b.logger.WarnContext(ctx, "potentiate memory failed", "id", id, "error", err)
	}
	return m, nil
}

func (b *Brain) Meta(ctx context.Context) (*BrainMeta, error) {
	all, err := b.store.AllMeta(ctx)
	if err != nil {
		return nil, fmt.Errorf("brain: read meta: %w", err)
	}
	return &BrainMeta{
		SchemaVersion: all[model.MetaKeySchemaVersion],
		CreatedAt:     all[model.MetaKeyCreatedAt],
		Description:   all[model.MetaKeyDescription],
		LastBackupAt:  all[model.MetaKeyLastBackupAt],
	}, nil
}

func (b *Brain) SetMeta(ctx context.Context, key, value string) error {
	return b.store.SetMeta(ctx, key, value)
}

type ActivityReport struct {
	MemoryCounts         map[string]int          `json:"memory_counts"`
	HumanCount           int                     `json:"human_count"`
	AgentCount           int                     `json:"agent_count"`
	PendingIngests       int                     `json:"pending_ingests"`
	UsersNeedingAnalysis int                     `json:"users_needing_analysis"`
	RecentConsolidations []ConsolidationLogEntry `json:"recent_consolidations,omitempty"`
	LLMConfigured        bool                    `json:"llm_configured"`
	LLMAvailable         bool                    `json:"llm_available"`
}

// Activity builds a health-check report (for daemon/agent verification).
func (b *Brain) Activity(ctx context.Context) (*ActivityReport, error) {
	r := &ActivityReport{
		MemoryCounts: make(map[string]int),
	}

	wc, _ := b.Working.Count(ctx)
	ec, _ := b.Episodic.Count(ctx)
	sc, _ := b.Semantic.Count(ctx)
	pc, _ := b.Procedural.Count(ctx)
	r.MemoryCounts["working"] = wc
	r.MemoryCounts["episodic"] = ec
	r.MemoryCounts["semantic"] = sc
	r.MemoryCounts["procedural"] = pc

	if agents, err := b.Social.ListAgents(ctx); err == nil {
		r.AgentCount = len(agents)
	}
	if humans, err := b.Social.ListHumans(ctx); err == nil {
		r.HumanCount = len(humans)
	}

	if pending, err := b.store.UnprocessedIngests(ctx, 0); err == nil {
		r.PendingIngests = len(pending)
	}

	if users, err := b.Social.HumansNeedingAnalysis(ctx); err == nil {
		r.UsersNeedingAnalysis = len(users)
	}

	if logs, err := b.store.RecentConsolidationLogs(ctx, 5); err == nil {
		r.RecentConsolidations = logs
	}

	r.LLMConfigured = b.cfg.llmBaseURL != "" && b.cfg.llmModel != ""
	if r.LLMConfigured {
		lc := llm.NewClient(b.cfg.llmBaseURL, b.cfg.llmModel)
		lc.APIKey = b.cfg.llmAPIKey
		r.LLMAvailable = lc.Available(ctx)
	}

	return r, nil
}

// RecordBackup sets last backup timestamp. Call after successfully copying the .brain file.
func (b *Brain) RecordBackup(ctx context.Context) error {
	return b.store.SetMeta(ctx, model.MetaKeyLastBackupAt,
		time.Now().UTC().Format(store.SQLiteTimeFmt))
}

// ValidationResult holds the result of read-only brain validation (schema, meta, row counts).
type ValidationResult struct {
	OK          bool
	Issues      []string
	SchemaVer   string
	CreatedAt   string
	TableCounts map[string]int64
}

// Validate runs read-only checks: required brain_meta keys, table existence, and row counts.
func (b *Brain) Validate(ctx context.Context) (*ValidationResult, error) {
	r := &ValidationResult{TableCounts: make(map[string]int64)}
	schemaVer, err := b.store.GetMeta(ctx, model.MetaKeySchemaVersion)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("schema_version: %v", err))
		return r, nil
	}
	r.SchemaVer = schemaVer
	if schemaVer == "" {
		r.Issues = append(r.Issues, "schema_version is missing")
	}
	createdAt, err := b.store.GetMeta(ctx, model.MetaKeyCreatedAt)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("created_at: %v", err))
		return r, nil
	}
	r.CreatedAt = createdAt
	if createdAt == "" {
		r.Issues = append(r.Issues, "created_at is missing")
	}
	tables := []string{"memories", "contacts", "interactions", "consolidation_log", "brain_meta", "schema_migrations", "episodic_archive"}
	for _, table := range tables {
		var n int64
		err := b.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&n)
		if err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("table %s: %v", table, err))
			continue
		}
		r.TableCounts[table] = n
	}
	r.OK = len(r.Issues) == 0
	return r, nil
}

func (b *Brain) MemoriesForUser(ctx context.Context, userID string, limit int) ([]Memory, error) {
	return b.store.MemoriesByUser(ctx, userID, limit)
}

func (b *Brain) ForgetMemory(ctx context.Context, id int64) error {
	return b.store.DeleteMemory(ctx, id)
}

// PinMemory sets salience to 1.0 and marks pinned (exempt from decay; e.g. project conventions).
func (b *Brain) PinMemory(ctx context.Context, id int64) error {
	return b.store.PinMemory(ctx, id)
}

func (b *Brain) UnpinMemory(ctx context.Context, id int64) error {
	return b.store.UnpinMemory(ctx, id)
}

// CorrectMemory updates content in-place (preserves salience, tags, retrieval history).
func (b *Brain) CorrectMemory(ctx context.Context, id int64, newContent string) error {
	return b.store.CorrectMemory(ctx, id, newContent)
}

// Timeline returns archived episodic memories (pre-semanticization), most recent first.
func (b *Brain) Timeline(ctx context.Context, userID string, limit int) ([]EpisodicArchive, error) {
	return b.store.Timeline(ctx, userID, limit)
}

func (b *Brain) ExplainMemory(ctx context.Context, id int64) (*store.MemoryExplanation, error) {
	return b.store.ExplainMemory(ctx, id)
}

// MergeContact reassigns memories and interactions from fromID to toID, then deletes fromID (e.g. reconcile user-unknown).
func (b *Brain) MergeContact(ctx context.Context, fromID, toID string) (int, error) {
	return b.store.MergeContact(ctx, fromID, toID)
}

// Encode stores a raw conversation turn in working memory (no LLM). Daemon later runs EncoderFunc for episode/fact extraction.
func (b *Brain) Encode(ctx context.Context, userID, userName, agent, project, userPrompt, agentResponse string) (int64, error) {
	content := "## User prompt\n" + userPrompt + "\n\n## Agent response\n" + agentResponse
	meta := map[string]string{"type": "conversation"}
	if userName != "" {
		meta["user_name"] = userName
	}
	if project != "" {
		meta["project"] = project
	}
	return b.Working.StoreForUserWithAgent(ctx, content, meta, userID, agent)
}

func (b *Brain) Consolidate(ctx context.Context) (*ConsolidationStats, error) {
	return b.consolidator.Run(ctx)
}

// ProcessEncodings runs unprocessed conversation turns through the batch encoder (one LLM call); deletes them after extraction. Returns nil if no encoder is configured.
func (b *Brain) ProcessEncodings(ctx context.Context) (*model.EncodingStats, error) {
	if b.cfg.batchEncoderFunc == nil {
		return nil, nil
	}

	stats := &model.EncodingStats{}
	b.logger.Info("processing encodings", "action", "process_encodings")

	t0 := time.Now()
	pending, err := b.store.UnprocessedIngests(ctx, 50)
	if err != nil {
		return stats, fmt.Errorf("process encodings: query: %w", err)
	}
	b.logger.Info("pending encodings loaded", "action", "process_encodings", "pending", len(pending), "duration_ms", time.Since(t0).Milliseconds())
	if len(pending) == 0 {
		b.logger.Info("no pending encodings", "action", "process_encodings")
		return stats, nil
	}

	t0 = time.Now()
	turns := make([]model.EncodingTurn, len(pending))
	for i, raw := range pending {
		userPrompt, agentResponse := parseConversationContent(raw.Content)
		turns[i] = model.EncodingTurn{
			UserPrompt:    userPrompt,
			AgentResponse: agentResponse,
			UserID:        raw.UserID,
			UserName:      raw.Metadata["user_name"],
			Project:       raw.Metadata["project"],
			Agent:         raw.Agent,
		}
	}
	ctx = llm.WithLogger(ctx, b.logger.With("subsystem", "ingest"))
	b.logger.Info("build turns", "action", "process_encodings", "duration_ms", time.Since(t0).Milliseconds())

	t0 = time.Now()
	results, err := b.cfg.batchEncoderFunc(ctx, turns)
	if err != nil {
		return stats, fmt.Errorf("batch encode: %w", err)
	}
	b.logger.Info("batch encoder returned", "action", "process_encodings", "duration_ms", time.Since(t0).Milliseconds())

	t0 = time.Now()
	// Apply results: result[i] uses pending[i] for user/project/agent context. If the LLM merged
	// turns, we have fewer results than pending — remaining pending are just consumed (deleted).
	// If the LLM split, we have more results than pending — extra results use the last pending's context.
	lastRaw := (*model.Memory)(nil)
	for i, raw := range pending {
		lastRaw = &raw
		if i < len(results) {
			b.applyEncodingResult(ctx, raw, &results[i], stats)
		}
		if err := b.store.DeleteMemory(ctx, raw.ID); err != nil {
			b.logger.Warn("delete processed encoding failed", "action", "process_encoding", "id", raw.ID, "error", err)
		}
		stats.Processed++
	}
	for i := len(pending); i < len(results); i++ {
		if lastRaw != nil {
			b.applyEncodingResult(ctx, *lastRaw, &results[i], stats)
		}
	}
	b.logger.Info("encoding processing completed", "action", "process_encodings",
		"processed", stats.Processed, "episodes", stats.Episodes,
		"facts", stats.Facts, "interactions", stats.Interactions,
		"errors", stats.Errors,
		"apply_duration_ms", time.Since(t0).Milliseconds())
	return stats, nil
}

// applyEncodingResult stores episode, facts, and interaction for one encoding result and updates stats.
func (b *Brain) applyEncodingResult(ctx context.Context, raw model.Memory, result *model.EncodingResult, stats *model.EncodingStats) {
	userID := raw.UserID
	userName := raw.Metadata["user_name"]
	project := raw.Metadata["project"]
	agent := raw.Agent
	baseMeta := map[string]string{"source": "encode"}
	if project != "" {
		baseMeta["project"] = project
	}
	if result.Episode != "" {
		_, err := b.Episodic.RecordWithImportance(ctx, model.Episode{
			Content:  result.Episode,
			Tags:     result.Tags,
			UserID:   userID,
			Agent:    agent,
			Metadata: baseMeta,
		}, salienceFromOutcome(result.Outcome))
		if err != nil {
			b.logger.Warn("store episode failed", "action", "process_encoding", "error", err)
		} else {
			stats.Episodes++
		}
	}
	for _, factText := range result.Facts {
		_, err := b.Semantic.Store(ctx, model.Fact{
			Content:  factText,
			Tags:     result.Tags,
			UserID:   userID,
			Agent:    agent,
			Metadata: baseMeta,
		})
		if err != nil {
			b.logger.Warn("store fact failed", "action", "process_encoding", "error", err)
		} else {
			stats.Facts++
		}
	}
	if userID != "" {
		_, err := b.Social.RecordInteraction(ctx, model.Interaction{
			ContactID:   userID,
			ContactName: userName,
			EntityType:  model.EntityTypeHuman,
			Type:        "conversation",
			Outcome:     result.Outcome,
			Notes:       result.Episode,
		})
		if err != nil {
			b.logger.Warn("record interaction failed", "action", "process_encoding", "error", err)
		} else {
			stats.Interactions++
		}
	}
}

// parseConversationContent splits content into user prompt and agent response (format from Brain.Encode).
func parseConversationContent(content string) (userPrompt, agentResponse string) {
	const promptHeader = "## User prompt\n"
	const responseHeader = "\n\n## Agent response\n"

	idx := strings.Index(content, responseHeader)
	if idx < 0 {
		return content, ""
	}

	userPrompt = strings.TrimPrefix(content, promptHeader)
	userPrompt = userPrompt[:strings.Index(userPrompt, responseHeader)]
	agentResponse = content[idx+len(responseHeader):]

	return userPrompt, agentResponse
}

func salienceFromOutcome(outcome string) float64 {
	switch outcome {
	case "success":
		return 0.7
	case "failure":
		return 0.6
	default:
		return 0.5
	}
}

// AnalyzeProfiles runs one profile pass (InferTraitsFunc per human with new data). Returns nil if no InferTraitsFunc.
func (b *Brain) AnalyzeProfiles(ctx context.Context) ([]ProfileAnalysis, error) {
	return b.profiler.Run(ctx)
}

func (b *Brain) startConsolidationLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancelConsolidation = cancel
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.cfg.consolidationInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.Consolidate(ctx)
			}
		}
	}()
}

func (b *Brain) startProfilerLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancelProfiler = cancel
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.cfg.profileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.AnalyzeProfiles(ctx)
			}
		}
	}()
}

type emergedSkillAdapter struct{ *Brain }

func (a emergedSkillAdapter) StoreEmergedSkill(ctx context.Context, goal string, steps []string) error {
	name := slugForGoal(goal)
	_, err := a.Procedural.StoreEmerging(ctx, model.Procedure{
		Name:        name,
		Description: goal,
		Steps:       steps,
		SuccessRate: 0.8,
	})
	if err != nil {
		a.logger.WarnContext(ctx, "store emerging skill failed", "goal", goal, "error", err)
		return err
	}
	a.logger.InfoContext(ctx, "stored emerging skill from consolidation", "name", name, "goal", goal, "steps", len(steps))
	return nil
}

// slugForGoal returns an underscore-separated identifier (e.g. "Add dark mode" -> "add_dark_mode").
func slugForGoal(goal string) string {
	s := slug.Make(goal)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "emerging_skill"
	}
	return s
}
