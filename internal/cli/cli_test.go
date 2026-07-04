// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/logfile"
)

func TestDefaultLogPathUsesTmp(t *testing.T) {
	if got := logFilePathFromArgs([]string{"version"}); got != logfile.DefaultPath {
		t.Fatalf("got default log path %q, want %q", got, logfile.DefaultPath)
	}
}

func TestLogFilePathOverrideBeforeAndAfterSubcommand(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "custom.log")

	for _, args := range [][]string{
		{"--log-file", logPath, "version"},
		{"version", "--log-file", logPath},
		{"version", "--log-file=" + logPath},
	} {
		if got := logFilePathFromArgs(args); got != logPath {
			t.Fatalf("args %v got log path %q, want %q", args, got, logPath)
		}
	}
}

func TestValidateAcceptsFlagsAfterPositionals(t *testing.T) {
	yamlPath := writeValidYAML(t)
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{
		"validate",
		yamlPath,
		"--log-file", logPath,
	}, VersionInfo{Version: "test", Commit: "abc", Date: "today"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("got exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validation") || !strings.Contains(stdout.String(), "passed") {
		t.Fatalf("expected validation output, got %q", stdout.String())
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}
}

func TestRootHelpIncludesCommands(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")
	var stdout, stderr bytes.Buffer

	code := runWithIO([]string{"--log-file", logPath, "--help"}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("got exit code %d, stderr: %s", code, stderr.String())
	}
	for _, command := range []string{"validate", "plan", "execute", "status", "result", "version"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("expected help output to contain %q, got %q", command, stdout.String())
		}
	}
	assertHelpCommandOrder(t, stdout.String(), []string{"validate", "plan", "execute", "status", "result", "version"})
}

func TestLogFileOverrideTruncatesPerInvocation(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")
	if err := os.WriteFile(logPath, []byte("stale log contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{"version", "--log-file", logPath}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("got exit code %d, stderr: %s", code, stderr.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "stale log contents") {
		t.Fatalf("expected log file to be truncated, got %q", string(data))
	}
	if !strings.Contains(string(data), "metric-sync invocation started") {
		t.Fatalf("expected invocation metadata in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "metric-sync invocation completed") {
		t.Fatalf("expected invocation completion in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "exit_code=0") {
		t.Fatalf("expected success exit code in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "duration=") {
		t.Fatalf("expected duration in log, got %q", string(data))
	}
}

func TestCommandFailurePrintsDebugLogAndLogsExitCode(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")
	missingPath := filepath.Join(t.TempDir(), "missing.yaml")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{"validate", missingPath, "--log-file", logPath}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("got exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "inspect "+missingPath) {
		t.Fatalf("expected user-facing error, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "debug log: "+logPath) {
		t.Fatalf("expected debug log path in stderr, got %q", stderr.String())
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "command failed") {
		t.Fatalf("expected command failure in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "metric-sync invocation completed") {
		t.Fatalf("expected invocation completion in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "exit_code=1") {
		t.Fatalf("expected failure exit code in log, got %q", string(data))
	}
	if !strings.Contains(string(data), "duration=") {
		t.Fatalf("expected duration in log, got %q", string(data))
	}
}

func assertHelpCommandOrder(t *testing.T, help string, commands []string) {
	t.Helper()

	lastIndex := -1
	for _, command := range commands {
		index := strings.Index(help, "\n  "+command)
		if index == -1 {
			t.Fatalf("expected help output to contain command line for %q, got %q", command, help)
		}
		if index <= lastIndex {
			t.Fatalf("expected %q after prior command in help output, got %q", command, help)
		}
		lastIndex = index
	}
}

func writeValidYAML(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metric-sync.yaml")
	data := []byte(`
schema_version: 1
sync_tag: cobra-test
warehouse_connection_id: 00000000-0000-0000-0000-000000000000
warehouse_metric_sources:
  - sync_id: source
    name: Source
    sql: SELECT USER_ID, EVENT_TIMESTAMP, VALUE FROM table
    timestamp_column: EVENT_TIMESTAMP
    subject_types:
      - name: User
        column_name: USER_ID
    measures:
      - sync_id: value
        name: value
        column_name: VALUE
        column_type: FLOAT
metrics:
  - sync_id: metric
    name: Metric
    metric_type: simple
    simple_metric_aggregation:
      operation: sum
      measure:
        warehouse_metric_source_sync_id: source
        measure_sync_id: value
      timeframe_start_value: 0
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
