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
	"reflect"
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

func TestSubmitUsesExplicitWarehouseConnectionWithoutLookup(t *testing.T) {
	for _, command := range []string{"plan", "execute"} {
		for _, omitted := range []bool{false, true} {
			name := "explicit"
			if omitted {
				name = "omitted"
			}
			t.Run(command+"/"+name, func(t *testing.T) {
				yamlPath := writeValidYAMLWithWarehouseConnection(t, "explicit-connection")
				if omitted {
					data, err := os.ReadFile(yamlPath)
					if err != nil {
						t.Fatal(err)
					}
					content := strings.ReplaceAll(string(data), "    additional_timestamp_columns:\n      - TRANSACTION_TIMESTAMP\n", "")
					content = strings.ReplaceAll(content, "      winsorization_strategy: nonzero\n", "")
					if err := os.WriteFile(yamlPath, []byte(content), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				var received map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if got := r.URL.Query().Get("plan"); (got == "true") != (command == "plan") {
						t.Errorf("unexpected plan query: %q", got)
					}
					if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
						t.Error(err)
					}
					writeOperation(t, w, "operation-id", "cobra-test", command, "queued")
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "api")
				t.Setenv("DD_APP_KEY", "app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, yamlPath, "--site", server.URL, "--no-poll", "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				if code != 0 {
					t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
				}
				if received == nil {
					t.Fatal("expected submit")
				}
				if received["warehouse_connection_id"] != "explicit-connection" {
					t.Fatalf("unexpected warehouse connection: %v", received)
				}
				source := received["warehouse_metric_sources"].([]any)[0].(map[string]any)
				agg := received["metrics"].([]any)[0].(map[string]any)["simple_metric_aggregation"].(map[string]any)
				if omitted {
					if _, ok := source["additional_timestamp_columns"]; ok {
						t.Fatal("expected omitted timestamps")
					}
					if _, ok := agg["winsorization_strategy"]; ok {
						t.Fatal("expected omitted strategy")
					}
				} else {
					if !reflect.DeepEqual(source["additional_timestamp_columns"], []any{"TRANSACTION_TIMESTAMP"}) {
						t.Fatalf("unexpected timestamps: %v", source)
					}
					if agg["winsorization_strategy"] != "nonzero" {
						t.Fatalf("unexpected strategy: %v", agg)
					}
				}
				if agg["winsor_upper_percentile"] != 0.99 {
					t.Fatalf("unexpected upper percentile: %v", agg)
				}
			})
		}
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
    sql: SELECT USER_ID, EVENT_TIMESTAMP, TRANSACTION_TIMESTAMP, VALUE FROM table
    timestamp_column: EVENT_TIMESTAMP
    additional_timestamp_columns:
      - TRANSACTION_TIMESTAMP
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
      winsor_upper_percentile: 0.99
      winsorization_strategy: nonzero
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

func TestCommandsRejectInvalidFieldsBeforeAPIRequest(t *testing.T) {
	example, err := os.ReadFile(writeValidYAML(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, old, replacement, path, message string }{
		{"strategy", "winsorization_strategy: nonzero", `winsorization_strategy: ""`, "metrics[0].simple_metric_aggregation.winsorization_strategy", "must be all_assigned_subjects or nonzero"},
		{"duplicate timestamp", "      - TRANSACTION_TIMESTAMP", "      - TRANSACTION_TIMESTAMP\n      - TRANSACTION_TIMESTAMP", "warehouse_metric_sources[0].additional_timestamp_columns[1]", "must be unique"},
		{"too many timestamps", "      - TRANSACTION_TIMESTAMP", "      - a\n      - b\n      - c\n      - d\n      - e", "warehouse_metric_sources[0].additional_timestamp_columns", "must contain at most 4"},
		{"raw measure ID", "measure_sync_id: value", "warehouse_metric_measure_id: raw-id", "metrics[0].simple_metric_aggregation.measure.warehouse_metric_measure_id", "not supported by the sync API"},
		{"wrong measure operation", "measure_sync_id: value", "kind: each_record", "metrics[0].simple_metric_aggregation.operation", "sum requires a user-defined measure"},
		{"unknown subject", "measure_sync_id: value", "subject_type_name: Missing", "metrics[0].simple_metric_aggregation.measure.subject_type_name", "does not match subject_types[].name"},
	} {
		for _, command := range []string{"validate", "plan", "execute"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "invalid.yaml")
				content := strings.Replace(string(example), tc.old, tc.replacement, 1)
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("invalid fields must fail before any API request")
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "test-api")
				t.Setenv("DD_APP_KEY", "test-app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, path, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				output := stdout.String() + stderr.String()
				if code != 1 || !strings.Contains(output, path) || !strings.Contains(output, tc.path) || !strings.Contains(output, tc.message) {
					t.Fatalf("exit %d, expected %s: %s containing %q: %s", code, path, tc.path, tc.message, output)
				}
			})
		}
	}
}

func TestMeasureSelectorsThroughCommands(t *testing.T) {
	tests := []struct {
		name, operation, selector string
		want                      model.MeasureRef
	}{
		{name: "rows", operation: "count", selector: "kind: each_record", want: model.MeasureRef{Kind: "each_record"}},
		{name: "subjects", operation: "countDistinctValue", selector: "subject_type_name: User", want: model.MeasureRef{SubjectTypeName: "User"}},
		{name: "named", operation: "sum", selector: "measure_sync_id: value", want: model.MeasureRef{MeasureSyncID: "value"}},
		{name: "external", operation: "countDistinctValue", selector: "subject_type_name: Customer\n        warehouse_metric_source_sync_tag: another-tag", want: model.MeasureRef{SubjectTypeName: "Customer", WarehouseMetricSourceSyncTag: "another-tag"}},
	}
	for _, command := range []string{"validate", "plan", "execute"} {
		for _, test := range tests {
			t.Run(command+"/"+test.name, func(t *testing.T) {
				yamlPath := writeValidYAMLWithWarehouseConnection(t, "connection")
				data, err := os.ReadFile(yamlPath)
				if err != nil {
					t.Fatal(err)
				}
				text := strings.NewReplacer("operation: sum", "operation: "+test.operation, "measure_sync_id: value", test.selector).Replace(string(data))
				if err := os.WriteFile(yamlPath, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
				var received []model.SyncConfig
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					var request model.SyncConfig
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					received = append(received, request)
					writeOperation(t, w, "operation-id", "cobra-test", command, "queued")
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "")
				t.Setenv("DD_APP_KEY", "")
				args := []string{command, yamlPath, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}
				if command != "validate" {
					t.Setenv("DD_API_KEY", "test-api")
					t.Setenv("DD_APP_KEY", "test-app")
					args = append(args, "--no-poll")
				}
				var stdout, stderr bytes.Buffer
				code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr)
				if code != 0 {
					t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
				}
				if command == "validate" {
					if len(received) != 0 {
						t.Fatal("local validation called the API")
					}
					if !strings.Contains(stdout.String(), "local checks") || !strings.Contains(stdout.String(), "Run plan") {
						t.Fatalf("missing local validation guidance: %s", &stdout)
					}
					return
				}
				if len(received) != 1 || len(received[0].Metrics) != 1 || received[0].Metrics[0].SimpleMetricAggregation == nil {
					t.Fatalf("expected one submitted aggregation, got %v", received)
				}
				want := test.want
				want.WarehouseMetricSourceSyncID = "source"
				if got := received[0].Metrics[0].SimpleMetricAggregation; got.Measure != want || got.Operation != test.operation {
					t.Fatalf("submitted aggregation %v, want operation %s and measure %v", got, test.operation, want)
				}
			})
		}
	}
}

func TestValidateCompleteMeasureExample(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{"validate", "../../examples/minimal.yaml", "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
	}
}
