// Package logger creates the application's structured JSON logger.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger with environment-appropriate verbosity.
func New(environment string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(environment, "development") || strings.EqualFold(environment, "test") {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
