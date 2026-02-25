package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glthr/brAIn/internal/brain"
)

type taskRunner struct {
	b           *brain.Brain
	logger      *slog.Logger
	maxRuntime  time.Duration
	taskTimeout time.Duration
	mu          sync.Mutex
	running     map[string]*taskState
	wg          sync.WaitGroup
	nextRunID   int64
}

type taskState struct {
	started time.Time
	runID   int64
	cancel  context.CancelFunc
}

func newTaskRunner(b *brain.Brain, logger *slog.Logger, maxRuntime, taskTimeout time.Duration) *taskRunner {
	return &taskRunner{
		b:           b,
		logger:      logger,
		maxRuntime:  maxRuntime,
		taskTimeout: taskTimeout,
		running:     make(map[string]*taskState),
	}
}

// start launches a daemon task in its own goroutine if another instance of the
// same task is not already running.
func (r *taskRunner) start(name string, ctx context.Context, fn func(context.Context, *brain.Brain, *slog.Logger)) {
	now := time.Now()

	r.mu.Lock()
	if state := r.running[name]; state != nil {
		elapsed := now.Sub(state.started)
		if r.maxRuntime > 0 && elapsed > r.maxRuntime {
			if r.logger != nil {
				r.logger.Warn("task exceeded max runtime; canceling",
					"task", name,
					"run_id", state.runID,
					"elapsed", elapsed.String(),
					"max_runtime", r.maxRuntime.String())
			}
			if state.cancel != nil {
				state.cancel()
			}
		} else {
			if r.logger != nil {
				r.logger.Info("task skipped; previous run still in progress",
					"task", name,
					"run_id", state.runID,
					"elapsed", elapsed.String())
			}
			r.mu.Unlock()
			return
		}
	}

	runID := atomic.AddInt64(&r.nextRunID, 1)
	runCtx := ctx
	var cancel context.CancelFunc
	if r.taskTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.taskTimeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	state := &taskState{
		started: now,
		runID:   runID,
		cancel:  cancel,
	}
	r.running[name] = state
	r.wg.Add(1)
	r.mu.Unlock()

	if r.logger != nil {
		fields := []any{
			"task", name,
			"run_id", runID,
			"max_runtime", r.maxRuntime.String(),
		}
		if r.taskTimeout > 0 {
			fields = append(fields,
				"timeout", r.taskTimeout.String(),
				"deadline", now.Add(r.taskTimeout).Format(time.RFC3339))
		} else {
			fields = append(fields, "timeout", "none")
		}
		r.logger.Info("task started", fields...)
	}

	go func() {
		defer r.wg.Done()
		defer func() {
			cancel()
			r.mu.Lock()
			if r.running[name] == state {
				delete(r.running, name)
			}
			r.mu.Unlock()
			if r.logger != nil {
				r.logger.Info("task completed",
					"task", name,
					"run_id", runID,
					"duration", time.Since(state.started).String())
			}
		}()

		taskLogger := r.logger
		if taskLogger != nil {
			taskLogger = taskLogger.With("task", name, "run_id", runID)
		}
		fn(runCtx, r.b, taskLogger)
	}()
}

func (r *taskRunner) wait() {
	r.wg.Wait()
}

// isEncodingCancelOrTimeout reports whether the error is a canceled or deadline-exceeded
// context, including when wrapped. Such errors are logged at INFO and retried next cycle.
func isEncodingCancelOrTimeout(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func runIngestProcessing(ctx context.Context, b *brain.Brain, logger *slog.Logger) {
	if logger != nil {
		logger.Info("encoding processing started", "action", "process_encodings", "timeout", contextDeadline(ctx))
	}
	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		if logger != nil {
			if isEncodingCancelOrTimeout(err) {
				logger.Info("encoding run canceled or timed out, will retry next cycle", "action", "process_encodings", "error", err)
			} else {
				logger.Error("encoding processing error", "action", "process_encodings", "error", err)
			}
		}
		return
	}
	if stats == nil {
		if logger != nil {
			logger.Info("encoding processing completed: no encoder configured", "action", "process_encodings")
		}
		return
	}
	if stats.Processed == 0 && stats.Errors == 0 {
		if logger != nil {
			logger.Info("encoding processing completed: no pending encodings", "action", "process_encodings")
		}
		return
	}
	if stats.Errors > 0 {
		if logger != nil {
			logger.Warn("encoding processing completed with errors",
				"action", "process_encodings",
				"processed", stats.Processed,
				"episodes", stats.Episodes,
				"facts", stats.Facts,
				"interactions", stats.Interactions,
				"errors", stats.Errors)
		}
		return
	}
	if logger != nil {
		logger.Info("encoding processing completed",
			"action", "process_encodings",
			"processed", stats.Processed,
			"episodes", stats.Episodes,
			"facts", stats.Facts,
			"interactions", stats.Interactions,
			"errors", stats.Errors)
	}
}

func runConsolidation(ctx context.Context, b *brain.Brain, logger *slog.Logger) {
	if logger != nil {
		logger.Info("consolidation started", "action", "run", "timeout", contextDeadline(ctx))
	}
	stats, err := b.Consolidate(ctx)
	if err != nil {
		if logger != nil {
			logger.Error("consolidation error", "action", "run", "error", err)
		}
		return
	}
	total := stats.Expired + stats.Decayed + stats.Transferred + stats.Forgotten + stats.Semanticized
	if total > 0 {
		if logger != nil {
			logger.Info("consolidation completed",
				"action", "run",
				"expired", stats.Expired,
				"decayed", stats.Decayed,
				"transferred", stats.Transferred,
				"forgotten", stats.Forgotten,
				"semanticized", stats.Semanticized)
		}
		return
	}
	if logger != nil {
		logger.Info("consolidation completed: no changes", "action", "run")
	}
}

func runProfiling(ctx context.Context, b *brain.Brain, logger *slog.Logger) {
	if logger != nil {
		logger.Info("profile analysis started", "action", "run", "timeout", contextDeadline(ctx))
	}
	results, err := b.AnalyzeProfiles(ctx)
	if err != nil {
		if logger != nil {
			logger.Error("profile analysis error", "action", "run", "error", err)
		}
		return
	}
	if len(results) == 0 {
		if logger != nil {
			logger.Info("profile analysis completed: no users need analysis", "action", "run")
		}
		return
	}
	for _, r := range results {
		if r.Skipped {
			if logger != nil {
				logger.Warn("profile analysis skipped",
					"action", "run",
					"user_id", r.UserID,
					"reason", r.SkipReason)
			}
		} else {
			updated := "unchanged"
			if r.SocialSchemaUpdated {
				updated = "updated"
			}
			if logger != nil {
				logger.Info("profile analysis completed for user",
					"action", "run",
					"user_id", r.UserID,
					"social_schema", updated,
					"new_memories", r.NewMemories)
			}
		}
	}
	if logger != nil {
		logger.Info("profile analysis completed", "action", "run", "users", len(results))
	}
}

func contextDeadline(ctx context.Context) string {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline.Format(time.RFC3339)
	}
	return "none"
}
