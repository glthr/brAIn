package brain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/glthr/brAIn/internal/daemon"
)

const defaultFallbackURL = "http://127.0.0.1:11434"

func TestMain(m *testing.M) {
	code := runWithManagedOllama(m)
	os.Exit(code)
}

func runWithManagedOllama(m *testing.M) int {
	model := os.Getenv("BRAIN_TEST_OLLAMA_MODEL")
	if model == "" {
		model = daemon.DefaultTestOllamaModel
		_ = os.Setenv("BRAIN_TEST_OLLAMA_MODEL", model)
	}

	logDir, err := os.MkdirTemp("", "brain-test-ollama-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "test harness: failed to create temp log dir: %v\n", err)
		return 1
	}
	logFile := filepath.Join(logDir, "ollama-serve.log")
	logFH, err := os.Create(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test harness: failed to create log file: %v\n", err)
		return 1
	}
	defer func() { _ = logFH.Close() }()

	// Require Ollama: start serve if needed or use existing; fail if unavailable.
	url, hostport, cmd, err := startManagedOllama(logFH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test harness: %v\n", err)
		fmt.Fprintf(os.Stderr, "test harness: tests require Ollama; install it and ensure 'ollama serve' can run or is already running.\n")
		return 1
	}

	managed := cmd != nil
	if managed {
		defer stopProcess(cmd)
	}

	if err := ensureModelAvailable(hostport, model, logFH); err != nil {
		fmt.Fprintf(os.Stderr, "test harness: %v\n", err)
		return 1
	}

	_ = os.Setenv("BRAIN_TEST_OLLAMA_URL", url)
	_ = os.Setenv("BRAIN_TEST_OLLAMA_MODEL", model)
	fmt.Fprintf(os.Stderr, "test harness: using Ollama %s model=%s\n", url, model)

	exitCode := m.Run()
	if exitCode == 0 && os.Getenv("BRAIN_TEST_KEEP_OLLAMA_LOGS") != "1" {
		_ = os.Remove(logFile)
		_ = os.Remove(logDir)
	} else {
		fmt.Fprintf(os.Stderr, "test harness: ollama logs at %s\n", logFile)
	}
	return exitCode
}

func startManagedOllama(logFH *os.File) (string, string, *exec.Cmd, error) {
	if directURL := strings.TrimSpace(os.Getenv("BRAIN_TEST_OLLAMA_URL")); directURL != "" {
		hostport := strings.TrimPrefix(strings.TrimPrefix(directURL, "http://"), "https://")
		if err := waitForOllama(directURL, 20*time.Second, nil); err != nil {
			return "", "", nil, fmt.Errorf("configured BRAIN_TEST_OLLAMA_URL %s not reachable: %w", directURL, err)
		}
		return directURL, hostport, nil, nil
	}

	fallbackURL := strings.TrimSpace(os.Getenv("BRAIN_TEST_OLLAMA_FALLBACK_URL"))
	if fallbackURL == "" {
		fallbackURL = defaultFallbackURL
	}
	hostport := strings.TrimPrefix(strings.TrimPrefix(fallbackURL, "http://"), "https://")

	// Prefer an already-running default Ollama instance.
	if err := waitForOllama(fallbackURL, 2*time.Second, nil); err == nil {
		fmt.Fprintf(os.Stderr, "test harness: using existing Ollama at %s\n", fallbackURL)
		return fallbackURL, hostport, nil, nil
	}

	if os.Getenv("BRAIN_TEST_DISABLE_OLLAMA_AUTOSTART") == "1" {
		return "", "", nil, fmt.Errorf("no reachable Ollama at %s (set BRAIN_TEST_OLLAMA_URL or run 'ollama serve'); tests require Ollama", fallbackURL)
	}

	cmd := exec.CommandContext(context.Background(), "ollama", "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+hostport)
	cmd.Stdout = logFH
	cmd.Stderr = logFH
	if err := cmd.Start(); err != nil {
		return "", "", nil, fmt.Errorf("failed to start ollama serve on %s: %w", hostport, err)
	}

	if err := waitForOllama(fallbackURL, 20*time.Second, cmd); err == nil {
		return fallbackURL, hostport, cmd, nil
	}

	stopProcess(cmd)
	// If another process grabbed the default port, use it if it responds.
	if logContains(logFH.Name(), "address already in use") || logContains(logFH.Name(), "bind: address already in use") {
		if err := waitForOllama(fallbackURL, 5*time.Second, nil); err == nil {
			fmt.Fprintf(os.Stderr, "test harness: using existing Ollama at %s\n", fallbackURL)
			return fallbackURL, hostport, nil, nil
		}
	}
	return "", "", nil, errors.New("unable to start ollama on default port and no reachable Ollama instance found")
}

func ensureModelAvailable(hostport, model string, logFH *os.File) error {
	ok, err := modelExists(hostport, model)
	if err == nil && ok {
		return nil
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "test harness: model presence check failed (%v); attempting pull anyway\n", err)
	}

	fmt.Fprintf(os.Stderr, "test harness: pulling model %s\n", model)
	if err := pullModel(hostport, model, logFH); err != nil {
		return fmt.Errorf("failed pulling model %s (see %s): %w", model, logFH.Name(), err)
	}

	// Ollama can take a moment to update the model list after pull completes.
	for i := 0; i < 10; i++ {
		ok, err := modelExists(hostport, model)
		if err == nil && ok {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Do not fail solely on list inconsistency if pull succeeded.
	fmt.Fprintf(os.Stderr, "test harness: pull completed; proceeding even though model list check is inconclusive\n")
	return nil
}

func modelExists(hostport, model string) (bool, error) {
	cmd := exec.CommandContext(context.Background(), "ollama", "list")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+hostport)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	want := strings.ToLower(model)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		got := strings.ToLower(fields[0])
		if got == want || strings.HasPrefix(got, want+":") {
			return true, nil
		}
	}
	return false, sc.Err()
}

func pullModel(hostport, model string, logFH *os.File) error {
	cmd := exec.CommandContext(context.Background(), "ollama", "pull", model)
	cmd.Env = append(os.Environ(), "OLLAMA_HOST="+hostport)
	cmd.Stdout = logFH
	cmd.Stderr = logFH
	return cmd.Run()
}

func waitForOllama(url string, timeout time.Duration, cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: 800 * time.Millisecond}

	for {
		if cmd != nil && cmd.Process != nil {
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				return fmt.Errorf("ollama process exited before ready")
			}
		}

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/tags", nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func logContains(path, needle string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), needle)
}
