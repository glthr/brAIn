package brain

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const defaultLogDir = ".brain"
const defaultLogFile = "brain.log"
const maxLogSizeMB = 10

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// openLogFile opens ~/.brain/brain.log with size-based rotation (lumberjack). On error returns (nil, nopLogger).
func openLogFile() (io.Closer, *slog.Logger) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nopLogger()
	}
	dir := filepath.Join(home, defaultLogDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nopLogger()
	}
	path := filepath.Join(dir, defaultLogFile)

	lj := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxLogSizeMB,
		MaxBackups: 2,
		MaxAge:     7,
		Compress:   false,
	}
	logger := slog.New(slog.NewTextHandler(lj, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return lj, logger
}
