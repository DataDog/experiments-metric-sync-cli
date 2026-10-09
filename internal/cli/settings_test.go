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
)

func TestCommandsPreserveMetricSettings(t *testing.T) {
	example, err := os.ReadFile("../../examples/settings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"plan", "execute"} {
		for _, omitted := range []bool{false, true} {
			name := "explicit"
			if omitted {
				name = "omitted"
			}
			t.Run(command+"/"+name, func(t *testing.T) {
				content := string(example)
				if omitted {
					content = strings.ReplaceAll(content, "    additional_timestamp_columns:\n      - FIRST_TRANSACTION_DATETIME_UTC\n", "")
					lines := strings.Split(content, "\n")
					var kept []string
					for _, line := range lines {
						if !strings.Contains(line, "winsorization_strategy:") {
							kept = append(kept, line)
						}
					}
					content = strings.Join(kept, "\n")
				}
				content += "\nwarehouse_connection_id: test-warehouse\n"
				path := filepath.Join(t.TempDir(), "settings.yaml")
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
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
					writeOperation(t, w, "settings-operation", "example-bookings", command, "queued")
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "test-api")
				t.Setenv("DD_APP_KEY", "test-app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, path, "--site", server.URL, "--no-poll", "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				if code != 0 {
					t.Fatalf("exit %d: %s", code, &stderr)
				}
				if received == nil {
					t.Fatal("no request received")
				}
				source := received["warehouse_metric_sources"].([]any)[0].(map[string]any)
				metrics := received["metrics"].([]any)
				simple := metrics[0].(map[string]any)["simple_metric_aggregation"].(map[string]any)
				ratio := metrics[1].(map[string]any)["ratio_metric_aggregation"].(map[string]any)
				numerator := ratio["numerator_aggregation"].(map[string]any)
				denominator := ratio["denominator_aggregation"].(map[string]any)
				if omitted {
					if _, ok := source["additional_timestamp_columns"]; ok {
						t.Fatal("omitted timestamps must use API default")
					}
					for _, agg := range []map[string]any{simple, numerator, denominator} {
						if _, ok := agg["winsorization_strategy"]; ok {
							t.Fatal("omitted strategy must use API default")
						}
					}
				} else {
					if !reflect.DeepEqual(source["additional_timestamp_columns"], []any{"FIRST_TRANSACTION_DATETIME_UTC"}) {
						t.Fatalf("timestamps changed: %v", source)
					}
					if simple["winsorization_strategy"] != "nonzero" || numerator["winsorization_strategy"] != "nonzero" || denominator["winsorization_strategy"] != "all_assigned_subjects" {
						t.Fatalf("strategies changed: %v", metrics)
					}
				}
				if simple["winsor_upper_percentile"] != 0.99 || numerator["winsor_upper_percentile"] != 0.99 {
					t.Fatalf("bounds changed: %v", metrics)
				}
			})
		}
	}
}

func TestCommandsRejectInvalidMetricSettingsBeforeAPIRequest(t *testing.T) {
	example, err := os.ReadFile("../../examples/settings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, old, replacement, path string }{
		{"strategy", "winsorization_strategy: nonzero", `winsorization_strategy: ""`, "metrics[0].simple_metric_aggregation.winsorization_strategy"},
		{"duplicate timestamp", "      - FIRST_TRANSACTION_DATETIME_UTC", "      - FIRST_TRANSACTION_DATETIME_UTC\n      - FIRST_TRANSACTION_DATETIME_UTC", "warehouse_metric_sources[0].additional_timestamp_columns[1]"},
		{"too many timestamps", "      - FIRST_TRANSACTION_DATETIME_UTC", "      - a\n      - b\n      - c\n      - d\n      - e", "warehouse_metric_sources[0].additional_timestamp_columns"},
	} {
		for _, command := range []string{"validate", "plan", "execute"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "invalid.yaml")
				content := strings.Replace(string(example), tc.old, tc.replacement, 1)
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("invalid settings must fail before any API request")
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "test-api")
				t.Setenv("DD_APP_KEY", "test-app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, path, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				output := stdout.String() + stderr.String()
				if code == 0 || !strings.Contains(output, path) || !strings.Contains(output, tc.path) {
					t.Fatalf("exit %d, expected file and field error: %s", code, output)
				}
			})
		}
	}
}
