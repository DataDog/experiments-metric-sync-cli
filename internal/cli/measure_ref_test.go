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
	for _, command := range []string{"validate", "plan", "execute"} {
		for _, test := range []struct {
			name, operation, selector, tag string
			want                           model.MeasureRef
		}{
			{name: "rows", operation: "count", selector: "kind: each_record", want: model.MeasureRef{Kind: "each_record"}},
			{name: "subjects", operation: "countDistinctValue", selector: "subject_type_name: User", want: model.MeasureRef{SubjectTypeName: "User"}},
			{name: "named", operation: "sum", selector: "measure_sync_id: value", want: model.MeasureRef{MeasureSyncID: "value"}},
			{name: "external", operation: "countDistinctValue", selector: "subject_type_name: Customer", tag: "another-tag", want: model.MeasureRef{SubjectTypeName: "Customer", WarehouseMetricSourceSyncTag: "another-tag"}},
		} {
			t.Run(command+"/"+test.name, func(t *testing.T) {
				yamlPath := writeValidYAMLWithWarehouseConnection(t, "connection")
				data, err := os.ReadFile(yamlPath)
				if err != nil {
					t.Fatal(err)
				}
				text := strings.Replace(string(data), "operation: sum", "operation: "+test.operation, 1)
				selector := test.selector
				if test.tag != "" {
					selector += "\n        warehouse_metric_source_sync_tag: " + test.tag
				}
				text = strings.Replace(text, "measure_sync_id: value", selector, 1)
				if err := os.WriteFile(yamlPath, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
				test.want.WarehouseMetricSourceSyncID = "source"
				var received []model.MeasureRef
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					var request model.SyncConfig
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if len(request.Metrics) != 1 || request.Metrics[0].SimpleMetricAggregation == nil {
						t.Error("missing aggregation")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					received = append(received, request.Metrics[0].SimpleMetricAggregation.Measure)
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
				if code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr); code != 0 {
					t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
				}
				if command == "validate" {
					if len(received) != 0 {
						t.Fatal("validate called the API")
					}
					if !strings.Contains(stdout.String(), "local checks") || !strings.Contains(stdout.String(), "Run plan") {
						t.Fatalf("missing local validation guidance: %s", &stdout)
					}
				} else if len(received) != 1 || received[0] != test.want {
					t.Fatalf("submitted references %v, want %v", received, test.want)
				}
			})
		}
	}
}

func TestInvalidMeasureRefStopsBeforeSubmit(t *testing.T) {
	for _, command := range []string{"validate", "plan", "execute"} {
		for _, selector := range []string{"warehouse_metric_measure_id: raw-id", "kind: each_record", "subject_type_name: Missing"} {
			t.Run(command+"/"+selector, func(t *testing.T) {
				yamlPath := writeValidYAML(t)
				data, err := os.ReadFile(yamlPath)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "measure_sync_id: value", selector, 1))
				if err := os.WriteFile(yamlPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Errorf("invalid YAML sent a request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "test-api")
				t.Setenv("DD_APP_KEY", "test-app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, yamlPath, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				if code != 1 || !strings.Contains(stdout.String(), "metrics[0].simple_metric_aggregation") {
					t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
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
