// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/logfile"
	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
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

func TestPlanFetchesWarehouseConnectionWhenYAMLOmitsIt(t *testing.T) {
	yamlPath := writeValidYAML(t)
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")
	var sawLookup bool
	var sawSubmit bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/unstable/ffe/warehouse-connections":
			sawLookup = true
			writeWarehouseConnections(t, w, "resolved-connection")
		case r.Method == http.MethodPost && r.URL.Path == "/api/unstable/ffe/metric-syncs":
			sawSubmit = true
			var request model.SyncConfig
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.WarehouseConnectionID != "resolved-connection" {
				t.Fatalf("got warehouse connection %q", request.WarehouseConnectionID)
			}
			writeOperation(t, w, "operation-id", "cobra-test", "plan", "queued")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{
		"plan",
		yamlPath,
		"--site", server.URL,
		"--no-poll",
		"--log-file", logPath,
	}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("got exit code %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if !sawLookup || !sawSubmit {
		t.Fatalf("expected lookup and submit, saw lookup=%v submit=%v", sawLookup, sawSubmit)
	}
}

func TestPlanUsesExplicitWarehouseConnectionWithoutLookup(t *testing.T) {
	yamlPath := writeValidYAMLWithWarehouseConnection(t, "explicit-connection")
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")
	var sawSubmit bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/unstable/ffe/warehouse-connections" {
			t.Fatal("did not expect warehouse connection lookup")
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		sawSubmit = true
		var request model.SyncConfig
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.WarehouseConnectionID != "explicit-connection" {
			t.Fatalf("got warehouse connection %q", request.WarehouseConnectionID)
		}
		writeOperation(t, w, "operation-id", "cobra-test", "plan", "queued")
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{
		"plan",
		yamlPath,
		"--site", server.URL,
		"--no-poll",
		"--log-file", logPath,
	}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("got exit code %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if !sawSubmit {
		t.Fatal("expected submit")
	}
}

func TestPlanErrorsWhenNoWarehouseConnectionsExist(t *testing.T) {
	yamlPath := writeValidYAML(t)
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/unstable/ffe/warehouse-connections" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeWarehouseConnections(t, w)
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{
		"plan",
		yamlPath,
		"--site", server.URL,
		"--no-poll",
		"--log-file", logPath,
	}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("got exit code %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no warehouse connection is configured for this organization") {
		t.Fatalf("expected missing warehouse connection error, got %q", stderr.String())
	}
}

func TestPlanErrorsWhenMultipleWarehouseConnectionsExist(t *testing.T) {
	yamlPath := writeValidYAML(t)
	logPath := filepath.Join(t.TempDir(), "metric-sync.log")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/unstable/ffe/warehouse-connections" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeWarehouseConnections(t, w, "one", "two")
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")

	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{
		"plan",
		yamlPath,
		"--site", server.URL,
		"--no-poll",
		"--log-file", logPath,
	}, VersionInfo{Version: "test"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("got exit code %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "multiple warehouse connections are configured for this organization") {
		t.Fatalf("expected multiple warehouse connection error, got %q", stderr.String())
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
	return writeValidYAMLWithWarehouseConnection(t, "")
}

func writeValidYAMLWithWarehouseConnection(t *testing.T, warehouseConnectionID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metric-sync.yaml")
	connectionLine := ""
	if warehouseConnectionID != "" {
		connectionLine = "warehouse_connection_id: " + warehouseConnectionID + "\n"
	}
	data := []byte(`
schema_version: 1
sync_tag: cobra-test
` + connectionLine + `
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

func writeWarehouseConnections(t *testing.T, w http.ResponseWriter, ids ...string) {
	t.Helper()
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{
			"id":   id,
			"type": "warehouse-connections",
			"attributes": map[string]any{
				"name":   "Warehouse " + id,
				"engine": "SNOWFLAKE",
			},
		})
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		t.Fatal(err)
	}
}

func writeOperation(t *testing.T, w http.ResponseWriter, id string, syncTag string, operation string, status string) {
	t.Helper()
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"id":   id,
			"type": "metric-sync-operations",
			"attributes": map[string]any{
				"metric_sync_id":  id,
				"sync_tag":        syncTag,
				"operation_type":  operation,
				"status":          status,
				"created_at":      "2026-01-01T00:00:00Z",
				"status_detail":   nil,
				"payload_hash":    nil,
				"started_at":      nil,
				"completed_at":    nil,
				"temporal_run_id": nil,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
}
