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
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/experiments/metric-syncs":
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
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/experiments/metric-syncs" {
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

func TestPublicV2SubmitLifecycle(t *testing.T) {
	outcomes := []struct {
		name         string
		planCode     int
		executeCode  int
		requestCount int
	}{
		{"success", 0, 0, 4},
		{"failed", 1, 1, 3},
		{"no-poll", 0, 0, 1},
		{"submit-error", 1, 1, 1},
		{"status-error", 1, 1, 2},
		{"result-error", 1, 1, 4},
		{"blocked", 0, 1, 4},
	}
	for _, operation := range []string{"plan", "execute"} {
		for _, outcome := range outcomes {
			t.Run(operation+"/"+outcome.name, func(t *testing.T) {
				var requests []string
				server := httptest.NewServer(submitLifecycleHandler(t, operation, outcome.name, &requests))
				defer server.Close()
				code, stdout, stderr := runSubmitLifecycle(t, server.URL, operation, outcome.name)
				wantCode := outcome.planCode
				if operation == "execute" {
					wantCode = outcome.executeCode
				}
				if code != wantCode || len(requests) != outcome.requestCount {
					t.Fatalf("code=%d want=%d requests=%v want count=%d stdout=%s stderr=%s", code, wantCode, requests, outcome.requestCount, stdout, stderr)
				}
				if outcome.name == "no-poll" && !strings.Contains(stdout, "operation-id") {
					t.Errorf("missing operation ID: %s", stdout)
				}
			})
		}
	}
}

func writeLifecycleYAML(t *testing.T) string {
	t.Helper()
	path := writeValidYAMLWithWarehouseConnection(t, "explicit-connection")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\noptions:\n  is_certified: false\n  upgrade_mode: by_id\n  force_delete: true\n")...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runSubmitLifecycle(t *testing.T, site, operation, outcome string) (int, string, string) {
	t.Helper()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")
	args := []string{operation, writeLifecycleYAML(t), "--site", site, "--idempotency-key", "caller-key", "--poll-interval", "1ms", "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}
	if outcome == "no-poll" {
		args = append(args, "--no-poll")
	}
	var stdout, stderr bytes.Buffer
	code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func submitLifecycleHandler(t *testing.T, operation, outcome string, requests *[]string) http.HandlerFunc {
	t.Helper()
	polls := 0
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.Path)
		if r.Header.Get("DD-API-KEY") != "api" || r.Header.Get("DD-APPLICATION-KEY") != "app" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing authentication or accept headers")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/experiments/metric-syncs":
			assertLifecycleSubmit(t, r, operation)
			if outcome == "submit-error" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeOperation(t, w, "operation-id", "cobra-test", operation, "queued")
		case "GET /api/v2/experiments/metric-syncs/operation-id":
			polls++
			writeLifecycleStatus(w, operation, outcome, polls)
		case "GET /api/v2/experiments/metric-syncs/operation-id/result":
			writeLifecycleResult(w, operation, outcome)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func assertLifecycleSubmit(t *testing.T, r *http.Request, operation string) {
	t.Helper()
	wantPlan := "false"
	if operation == "plan" {
		wantPlan = "true"
	}
	if r.URL.Query().Get("plan") != wantPlan || r.URL.Query().Get("is_certified") != "false" || r.URL.Query().Get("upgrade_mode") != "by_id" || r.URL.Query().Get("force_delete") != "true" {
		t.Errorf("unexpected options: %s", r.URL.RawQuery)
	}
	if r.Header.Get("Idempotency-Key") != "caller-key" || r.Header.Get("Content-Type") != "application/json" {
		t.Error("missing submit headers")
	}
	assertLifecyclePayload(t, r)
}

func assertLifecyclePayload(t *testing.T, r *http.Request) {
	t.Helper()
	var request model.SyncConfig
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Error(err)
		return
	}
	if request.SchemaVersion != 1 || request.SyncTag != "cobra-test" || request.WarehouseConnectionID != "explicit-connection" || len(request.WarehouseMetricSources) != 1 || len(request.Metrics) != 1 {
		t.Errorf("unexpected payload: %#v", request)
		return
	}
	source, metric := request.WarehouseMetricSources[0], request.Metrics[0]
	if len(source.Measures) != 1 || metric.SimpleMetricAggregation == nil {
		t.Errorf("missing measure or aggregation: %#v", request)
		return
	}
	if source.SyncID != "source" || source.Measures[0].SyncID != "value" || metric.SyncID != "metric" || metric.SimpleMetricAggregation.Measure.MeasureSyncID != "value" {
		t.Error("caller sync IDs changed")
	}
}

func lifecycleTerminalStatus(operation, outcome string) string {
	if outcome == "failed" {
		return "failed"
	}
	if operation == "plan" {
		return "planned"
	}
	return "success"
}

func writeLifecycleStatus(w http.ResponseWriter, operation, outcome string, polls int) {
	if outcome == "status-error" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	status := lifecycleTerminalStatus(operation, outcome)
	if polls == 1 {
		status = "running"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"id":         "operation-id",
		"attributes": map[string]any{"status": status, "operation_type": operation},
	}})
}

func writeLifecycleResult(w http.ResponseWriter, operation, outcome string) {
	if outcome == "result-error" {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	blocked := []map[string]any{}
	if outcome == "blocked" {
		blocked = append(blocked, map[string]any{"sync_id": "metric", "reason": "in use"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"id": "operation-id",
		"attributes": map[string]any{
			"status":  lifecycleTerminalStatus(operation, outcome),
			"plan":    operation == "plan",
			"created": map[string]any{"metrics": []map[string]any{{"sync_id": "metric", "id": "metric-id"}}},
			"blocked": map[string]any{"metrics": blocked},
		},
	}})
}

func TestPublicV2StatusAndResultCommands(t *testing.T) {
	for _, command := range []string{"status", "result"} {
		for _, response := range []string{"ready", "pending", "error"} {
			t.Run(command+"/"+response, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := "/api/v2/experiments/metric-syncs/operation-id"
					if command == "result" {
						path += "/result"
					}
					if r.Method != http.MethodGet || r.URL.Path != path {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					}
					if response == "error" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					if response == "pending" && command == "result" {
						w.Header().Set("Retry-After", "2")
						w.WriteHeader(http.StatusAccepted)
					}
					_, _ = w.Write([]byte(`{"data":{"id":"operation-id","attributes":{"status":"planned","plan":true,"message":"not ready","created":{"metrics":[{"sync_id":"metric","id":"metric-id"}]}}}}`))
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "api")
				t.Setenv("DD_APP_KEY", "app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, "operation-id", "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				want := 0
				if response == "error" {
					want = 1
				}
				if response == "pending" && command == "result" {
					want = 2
				}
				if code != want {
					t.Fatalf("code=%d want=%d stdout=%s stderr=%s", code, want, stdout.String(), stderr.String())
				}
				if want == 0 && !strings.Contains(stdout.String(), "operation-id") {
					t.Errorf("missing decoded ID: %s", stdout.String())
				}
			})
		}
	}
}
