// Package daemon: start and stop ollama serve when the daemon uses an Ollama URL
// and no server is already running. Logs from the subprocess are written to
// ~/.brain/logs/ollama.log and forwarded to the daemon logger.
package daemon

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	ollamaReachCheckInterval = 250 * time.Millisecond
	ollamaStartTimeout       = 60 * time.Second
	ollamaQuickCheckTimeout  = 2 * time.Second
)

// WaitForOllama returns nil when baseURL responds to /api/tags with 200.
// If cmd is non-nil, it checks that the process is still running each loop.
func WaitForOllama(ctx context.Context, baseURL string, timeout time.Duration, cmd *exec.Cmd) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	u := strings.TrimSuffix(baseURL, "/") + "/api/tags"

	for {
		if cmd != nil && cmd.Process != nil {
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ollamaReachCheckInterval):
		}
	}
}

// hostportFromURL returns host:port from a URL like http://localhost:11434 or http://127.0.0.1:11434.
func hostportFromURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

// StartOllamaIfNeeded ensures an Ollama server is running at the given URL (e.g. http://localhost:11434).
// If the URL is already reachable within a short check, it returns (nil, nil).
// Otherwise it starts "ollama serve" with OLLAMA_HOST set, captures its stdout/stderr to
// ~/.brain/logs/ollama.log and to the logger, waits for the server to respond, and returns
// the *exec.Cmd so the caller can defer StopOllama(cmd). Only call when IsLocalOllamaURL(url).
func StartOllamaIfNeeded(ctx context.Context, baseURL string, logger *slog.Logger) (*exec.Cmd, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Prefer an already-running server.
	quickCtx, quickCancel := context.WithTimeout(ctx, ollamaQuickCheckTimeout)
	err := WaitForOllama(quickCtx, baseURL, ollamaQuickCheckTimeout, nil)
	quickCancel()
	if err == nil {
		return nil, nil
	}

	hostport, err := hostportFromURL(baseURL)
	if err != nil {
		return nil, err
	}

	logDir, err := ollamaLogDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, "ollama.log")
	if errMkdir := os.MkdirAll(logDir, 0o755); errMkdir != nil {
		return nil, errMkdir
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	lineLogger := &lineForwarder{logger: logger, key: "ollama"}
	tee := io.MultiWriter(logFile, lineLogger)

	// Use WithoutCancel so the subprocess is not killed when ctx (startup timeout) is cancelled.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "ollama", "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+hostport)
	cmd.Stdout = tee
	cmd.Stderr = tee
	if errStart := cmd.Start(); errStart != nil {
		_ = logFile.Close()
		return nil, errStart
	}

	logger.Info("started ollama serve", "hostport", hostport, "log", logPath)

	waitCtx, waitCancel := context.WithTimeout(ctx, ollamaStartTimeout)
	err = WaitForOllama(waitCtx, baseURL, ollamaStartTimeout, cmd)
	waitCancel()

	if err != nil {
		_ = stopOllama(cmd)
		_ = logFile.Close()
		return nil, err
	}

	// Leave logFile open so the tee keeps writing ollama stdout/stderr to ~/.brain/logs/ollama.log.
	return cmd, nil
}

func ollamaLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".brain", "logs"), nil
}

// lineForwarder implements io.Writer and forwards each line to the logger.
type lineForwarder struct {
	logger *slog.Logger
	key    string
	mu     sync.Mutex
	buf    []byte
}

func (w *lineForwarder) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(w.buf[:idx+1]), "\r\n")
		w.buf = w.buf[idx+1:]
		if line != "" {
			w.logger.Info("ollama log", "source", w.key, "line", line)
		}
	}
	return len(p), nil
}

// StopOllama terminates the ollama serve process started by StartOllamaIfNeeded.
func StopOllama(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func stopOllama(cmd *exec.Cmd) error {
	StopOllama(cmd)
	return nil
}
