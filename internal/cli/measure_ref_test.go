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

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestMeasureSelectorsThroughCommands(t *testing.T) {
	tests := []struct {
		name, operation, selector string
		want                      model.MeasureRef
		wantError                 bool
	}{
		{name: "rows", operation: "count", selector: "kind: each_record", want: model.MeasureRef{Kind: "each_record"}},
		{name: "subjects", operation: "countDistinctValue", selector: "subject_type_name: User", want: model.MeasureRef{SubjectTypeName: "User"}},
		{name: "named", operation: "sum", selector: "measure_sync_id: value", want: model.MeasureRef{MeasureSyncID: "value"}},
		{name: "external", operation: "countDistinctValue", selector: "subject_type_name: Customer\n        warehouse_metric_source_sync_tag: another-tag", want: model.MeasureRef{SubjectTypeName: "Customer", WarehouseMetricSourceSyncTag: "another-tag"}},
		{name: "raw ID", operation: "sum", selector: "warehouse_metric_measure_id: raw-id", wantError: true},
		{name: "wrong operation", operation: "sum", selector: "kind: each_record", wantError: true},
		{name: "unknown subject", operation: "sum", selector: "subject_type_name: Missing", wantError: true},
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
				wantCode := 0
				if test.wantError {
					wantCode = 1
					if !strings.Contains(stdout.String(), "metrics[0].simple_metric_aggregation") {
						t.Errorf("missing validation error: %s", &stdout)
					}
				}
				if code != wantCode {
					t.Fatalf("exit %d, want %d, stdout: %s stderr: %s", code, wantCode, &stdout, &stderr)
				}
				if command == "validate" || test.wantError {
					if len(received) != 0 {
						t.Fatal("local validation called the API")
					}
					if !test.wantError && (!strings.Contains(stdout.String(), "local checks") || !strings.Contains(stdout.String(), "Run plan")) {
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
