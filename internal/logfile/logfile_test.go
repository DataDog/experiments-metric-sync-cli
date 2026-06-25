package logfile

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerRedactsCredentials(t *testing.T) {
	t.Setenv("DD_API_KEY", "api-secret")
	t.Setenv("DD_APP_KEY", "app-secret")
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")

	logger, err := New(logPath, []string{"validate"}, VersionInfo{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("request api-secret", slog.String("header", "app-secret"))
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if strings.Contains(contents, "api-secret") || strings.Contains(contents, "app-secret") {
		t.Fatalf("expected secrets to be redacted, got %q", contents)
	}
	if !strings.Contains(contents, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", contents)
	}
}

func TestLoggerCreatesPrivateFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")

	logger, err := New(logPath, nil, VersionInfo{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("got permissions %o, want 0600", got)
	}
}
