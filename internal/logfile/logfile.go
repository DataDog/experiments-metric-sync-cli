package logfile

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var DefaultPath = filepath.Join(os.TempDir(), "metric-sync-debug.log")

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

type Logger struct {
	file    *os.File
	path    string
	logger  *slog.Logger
	handler slog.Handler
}

func New(path string, args []string, version VersionInfo) (*Logger, error) {
	if path == "" {
		path = DefaultPath
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}

	handler := &redactingHandler{
		next: slog.NewTextHandler(file, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
		secrets: []string{
			os.Getenv("DD_API_KEY"),
			os.Getenv("DD_APP_KEY"),
		},
	}
	logger := slog.New(handler)
	result := &Logger{
		file:    file,
		path:    path,
		logger:  logger,
		handler: handler,
	}
	result.Info(
		"metric-sync invocation started",
		slog.Time("time", time.Now()),
		slog.String("version", version.Version),
		slog.String("commit", version.Commit),
		slog.String("date", version.Date),
		slog.Any("args", args),
	)
	if wd, err := os.Getwd(); err == nil {
		result.Info("working directory", slog.String("path", wd))
	}
	return result, nil
}

func (l *Logger) Complete(exitCode int, duration time.Duration) {
	if l == nil || l.logger == nil {
		return
	}
	l.Info(
		"metric-sync invocation completed",
		slog.Int("exit_code", exitCode),
		slog.Duration("duration", duration),
	)
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Info(message string, attrs ...slog.Attr) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, message, attrs...)
}

func (l *Logger) Error(message string, attrs ...slog.Attr) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.LogAttrs(context.Background(), slog.LevelError, message, attrs...)
}

type redactingHandler struct {
	next    slog.Handler
	secrets []string
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, h.redactString(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, h.redactAttr(attr))
	}
	return &redactingHandler{
		next:    h.next.WithAttrs(redacted),
		secrets: h.secrets,
	}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{
		next:    h.next.WithGroup(name),
		secrets: h.secrets,
	}
}

func (h *redactingHandler) redactAttr(attr slog.Attr) slog.Attr {
	attr.Value = h.redactValue(attr.Value)
	return attr
}

func (h *redactingHandler) redactValue(value slog.Value) slog.Value {
	switch value.Kind() {
	case slog.KindString:
		return slog.StringValue(h.redactString(value.String()))
	case slog.KindGroup:
		attrs := value.Group()
		redacted := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			redacted = append(redacted, h.redactAttr(attr))
		}
		return slog.GroupValue(redacted...)
	default:
		return value
	}
}

func (h *redactingHandler) redactString(value string) string {
	for _, secret := range h.secrets {
		if secret == "" {
			continue
		}
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}
