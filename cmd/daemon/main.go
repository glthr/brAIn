// Command brain-daemon is the background worker that runs memory consolidation,
// encoding extraction, and user profile inference on a timer. It requires an
// LLM connection (Ollama by default) and is the only process that performs
// LLM calls against the brain.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/glthr/brAIn/internal/brain"
	"github.com/glthr/brAIn/internal/daemon"
)

func fatalf(logger *slog.Logger, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logger != nil {
		logger.Error("fatal error", "error", msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s", msg)
	}
	os.Exit(1)
}

// redirectOutputToLog opens ~/.brain/daemon.log in append mode and points
// os.Stdout and os.Stderr at it so restarts accumulate instead of overwriting.
// It also performs a basic size-based rotation.
func redirectOutputToLog() (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	logPath := filepath.Join(home, ".brain", "daemon.log")
	if errMkdir := os.MkdirAll(filepath.Dir(logPath), 0o755); errMkdir != nil {
		return "", false, errMkdir
	}

	rotated, rotateErr := rotateDaemonLog(logPath)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return logPath, rotated, err
	}
	os.Stdout = f
	os.Stderr = f
	return logPath, rotated, rotateErr
}

func captureOllamaLogsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BRAIN_CAPTURE_OLLAMA_LOGS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ollamaLogPath() string {
	if path := strings.TrimSpace(os.Getenv("BRAIN_OLLAMA_LOG_PATH")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ollama", "logs", "server.log")
}

func startOllamaLogCapture(ctx context.Context, logger *slog.Logger) {
	if !captureOllamaLogsEnabled() {
		return
	}
	path := ollamaLogPath()
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}

	go func() {
		var offset int64
		var f *os.File
		var reader *bufio.Reader

		openFile := func() bool {
			if f != nil {
				_ = f.Close()
			}
			file, err := os.Open(path)
			if err != nil {
				return false
			}
			f = file
			if off, err := f.Seek(0, io.SeekEnd); err == nil {
				offset = off
			} else {
				offset = 0
			}
			reader = bufio.NewReader(f)
			return true
		}

		if !openFile() {
			return
		}
		defer func() { _ = f.Close() }()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err == nil {
				offset += int64(len(line))
				if logger != nil {
					logger.Info("ollama log", "source", "ollama", "line", strings.TrimRight(line, "\n"))
				}
				continue
			}
			if err != io.EOF {
				time.Sleep(500 * time.Millisecond)
				continue
			}

			info, statErr := os.Stat(path)
			if statErr == nil && info.Size() < offset {
				if !openFile() {
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// defaultBrainFile returns ~/.brain/agent.brain.
func defaultBrainFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".brain", "agent.brain"), nil
}

func main() {
	logPath, rotated, rotateErr := redirectOutputToLog()
	cfg := daemon.LoadConfig()
	logLevel := daemon.ParseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	brainFile, err := defaultBrainFile()
	if err != nil {
		fatalf(logger, "%v\n", err)
	}

	if rotateErr != nil {
		logger.Warn("daemon log rotation failed", "error", rotateErr)
	}
	if rotated {
		logger.Info("daemon log rotated", "path", logPath)
	}

	execPath, execErr := os.Executable()
	if execErr != nil {
		execPath = ""
	}

	state, stateErr := loadDaemonState()
	if stateErr != nil {
		logger.Warn("failed to load daemon state", "error", stateErr)
	}
	state.StartCount++
	state.LastStart = time.Now()
	if errSave := saveDaemonState(state); errSave != nil {
		logger.Warn("failed to persist daemon state", "error", errSave)
	}

	startTime := time.Now()

	// Auto-update on start: build from source and replace binary, then exit so the service restarts with the new binary.
	if cfg.AutoUpdateOnStart && cfg.SourcePath != "" && execPath != "" {
		if errUpdate := daemon.BuildAndReplace(cfg.SourcePath, execPath); errUpdate != nil {
			logger.Error("auto-update on start failed", "error", errUpdate)
		} else {
			logger.Info("auto-update completed; exiting so service restarts")
			os.Exit(0)
		}
	}

	// LLM connection is mandatory -- fail early with a helpful error if not configured.
	if cfg.LLMUrl == "" || cfg.LLMModel == "" {
		fatalf(logger, "LLM connection is mandatory (prerequisite). Configure LLM settings:\n\n"+
			"  1. Create/edit ~/.brain/config:\n"+
			"     llm_url = http://localhost:11434\n"+
			"     llm_model = qwen3:4b\n\n"+
			"  2. Or set environment variables:\n"+
			"     export BRAIN_LLM_URL=http://localhost:11434\n"+
			"     export BRAIN_LLM_MODEL=qwen3:4b\n\n"+
			"The brain daemon cannot function without an LLM connection.\n")
	}

	// For Ollama endpoints: start "ollama serve" only when local, then ensure all required models are available.
	if daemon.IsOllamaURL(cfg.LLMUrl) {
		if daemon.IsLocalOllamaURL(cfg.LLMUrl) {
			startCtx, startCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			ollamaCmd, errOllama := daemon.StartOllamaIfNeeded(startCtx, cfg.LLMUrl, logger)
			startCancel()
			if errOllama != nil {
				fatalf(logger, "ollama not reachable and failed to start: %v\n", errOllama)
			}
			if ollamaCmd != nil {
				defer daemon.StopOllama(ollamaCmd)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		models := daemon.RequiredOllamaModels(cfg)
		for _, model := range models {
			if ensureErr := daemon.EnsureModelAvailable(ctx, cfg.LLMUrl, model); ensureErr != nil {
				fatalf(logger, "failed to ensure model %s is available: %v\n", model, ensureErr)
			}
		}
	}

	opts := []brain.Option{
		brain.WithConsolidationInterval(0),
		brain.WithServiceMode(),
		brain.WithLogger(logger),
		brain.WithLLM(cfg.LLMUrl, cfg.LLMModel, cfg.LLMApiKey),
	}
	if cfg.EmbedModel != "" {
		opts = append(opts, brain.WithEmbeddingModel(cfg.EmbedModel))
	}

	b, err := brain.New(brainFile, opts...)
	if err != nil {
		fatalf(logger, "open brain: %v\n", err)
	}
	defer b.Close()

	// Verify the model responds before entering the main loop (fail fast).
	// This is the only place in the codebase where LLM reachability is checked.
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 60*time.Second)
	if err := b.CheckModel(checkCtx); err != nil {
		checkCancel()
		fatalf(logger, "model check failed: %v\n", err)
	}
	checkCancel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startOllamaLogCapture(ctx, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR2)

	logger.Info("brain daemon started",
		"pid", os.Getpid(),
		"restart_count", state.StartCount,
		"brain", brainFile,
		"config", daemon.ConfigFilePath(),
		"log_level", logLevel.String(),
		"llm_url", cfg.LLMUrl,
		"llm_model", cfg.LLMModel,
		"embed_model", cfg.EmbedModel,
		"ingest_interval", (cfg.ConsolidationInterval / 2).String(),
		"consolidation_interval", cfg.ConsolidationInterval.String(),
		"profile_interval", cfg.ProfileInterval.String())

	// Run all cycles once immediately on start.
	runner := newTaskRunner(b, logger, taskMaxRuntime(cfg), taskTimeout())
	runner.start("encoding processing", ctx, runIngestProcessing)
	runner.start("consolidation", ctx, runConsolidation)
	runner.start("profile analysis", ctx, runProfiling)

	// Ingest runs more frequently than consolidation so that raw conversation
	// turns are extracted promptly and available for profile analysis.
	ingestTicker := time.NewTicker(cfg.ConsolidationInterval / 2)
	defer ingestTicker.Stop()

	consolidationTicker := time.NewTicker(cfg.ConsolidationInterval)
	defer consolidationTicker.Stop()

	profileTicker := time.NewTicker(cfg.ProfileInterval)
	defer profileTicker.Stop()

	for {
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGUSR2 {
				// On-demand update: build from source, replace binary, exit so service restarts.
				if cfg.SourcePath != "" && execPath != "" {
					logger.Info("update requested", "signal", sig.String(), "source_path", cfg.SourcePath)
					if err := daemon.BuildAndReplace(cfg.SourcePath, execPath); err != nil {
						logger.Error("update on SIGUSR2 failed", "error", err)
					} else {
						logger.Info("update completed; exiting so service restarts")
						os.Exit(0)
					}
				} else {
					logger.Warn("update skipped: set source_path in ~/.brain/config (or BRAIN_SOURCE_PATH)")
				}
				continue
			}
			logger.Info("brain daemon stopping",
				"signal", sig.String(),
				"pid", os.Getpid(),
				"restart_count", state.StartCount,
				"uptime", time.Since(startTime).String())
			cancel()
			runner.wait()
			return
		case <-ctx.Done():
			logger.Info("brain daemon context done",
				"pid", os.Getpid(),
				"restart_count", state.StartCount,
				"uptime", time.Since(startTime).String())
			runner.wait()
			return
		case <-ingestTicker.C:
			runner.start("encoding processing", ctx, runIngestProcessing)
		case <-consolidationTicker.C:
			runner.start("consolidation", ctx, runConsolidation)
		case <-profileTicker.C:
			runner.start("profile analysis", ctx, runProfiling)
		}
	}
}

type daemonState struct {
	StartCount int       `json:"start_count"`
	LastStart  time.Time `json:"last_start"`
}

func daemonStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".brain", "daemon.state.json")
}

func loadDaemonState() (daemonState, error) {
	path := daemonStatePath()
	if path == "" {
		return daemonState{}, fmt.Errorf("cannot determine home directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return daemonState{}, nil
		}
		return daemonState{}, err
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return daemonState{}, err
	}
	return state, nil
}

func saveDaemonState(state daemonState) error {
	path := daemonStatePath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func rotateDaemonLog(logPath string) (bool, error) {
	maxBytes := daemonLogMaxBytes()
	if maxBytes <= 0 {
		return false, nil
	}
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Size() < maxBytes {
		return false, nil
	}
	rotatedName := fmt.Sprintf("%s.%s", logPath, time.Now().Format("20060102-150405"))
	if err := os.Rename(logPath, rotatedName); err != nil {
		return false, err
	}
	if err := pruneDaemonLogs(filepath.Dir(logPath), filepath.Base(logPath), daemonLogMaxBackups()); err != nil {
		return true, err
	}
	return true, nil
}

func daemonLogMaxBytes() int64 {
	const defaultMaxMB = 10
	if v := strings.TrimSpace(os.Getenv("BRAIN_DAEMON_LOG_MAX_MB")); v != "" {
		if mb, err := strconv.Atoi(v); err == nil && mb > 0 {
			return int64(mb) * 1024 * 1024
		}
	}
	return int64(defaultMaxMB) * 1024 * 1024
}

func daemonLogMaxBackups() int {
	const defaultMax = 5
	if v := strings.TrimSpace(os.Getenv("BRAIN_DAEMON_LOG_MAX_BACKUPS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMax
}

func pruneDaemonLogs(dir, base string, maxBackups int) error {
	if maxBackups <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var backups []os.DirEntry
	prefix := base + "."
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			backups = append(backups, entry)
		}
	}
	if len(backups) <= maxBackups {
		return nil
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() < backups[j].Name()
	})
	for _, entry := range backups[:len(backups)-maxBackups] {
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
	return nil
}

func taskMaxRuntime(cfg *daemon.Config) time.Duration {
	if v := strings.TrimSpace(os.Getenv("BRAIN_DAEMON_TASK_MAX_RUNTIME")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	if cfg == nil {
		return 0
	}
	return cfg.ConsolidationInterval * 2
}

func taskTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("BRAIN_DAEMON_TASK_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 0
}
