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
)

func TestUpgradeByNameThroughCommands(t *testing.T) {
	for _, command := range []string{"validate", "plan", "execute"} {
		for _, tc := range []struct {
			name, sourceField, metricField string
			wantSource, wantMetric         bool
		}{
			{name: "both", sourceField: "    upgrade_by_name: true\n", metricField: "    upgrade_by_name: true\n", wantSource: true, wantMetric: true},
			{name: "source only", sourceField: "    upgrade_by_name: true\n", wantSource: true},
			{name: "metric only", metricField: "    upgrade_by_name: true\n", wantMetric: true},
			{name: "explicit false", sourceField: "    upgrade_by_name: false\n", metricField: "    upgrade_by_name: false\n"},
			{name: "omitted"},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				yamlPath := writeValidYAMLWithWarehouseConnection(t, "connection")
				data, err := os.ReadFile(yamlPath)
				if err != nil {
					t.Fatal(err)
				}
				content := strings.NewReplacer(
					"sync_tag: cobra-test\n", "sync_tag: cobra-test\noptions:\n  upgrade_mode: by_name\n",
					"    name: Source\n", "    name: Source\n"+tc.sourceField,
					"    name: Metric\n", "    name: Metric\n"+tc.metricField,
				).Replace(string(data))
				if err := os.WriteFile(yamlPath, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				var received map[string][]map[string]json.RawMessage
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if got := r.URL.Query().Get("upgrade_mode"); got != "by_name" {
						t.Errorf("upgrade_mode = %q, want by_name", got)
					}
					if got := r.URL.Query().Get("plan"); (got == "true") != (command == "plan") {
						t.Errorf("unexpected plan query: %q", got)
					}
					var body struct {
						Sources []map[string]json.RawMessage `json:"warehouse_metric_sources"`
						Metrics []map[string]json.RawMessage `json:"metrics"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					received = map[string][]map[string]json.RawMessage{"warehouse_metric_sources": body.Sources, "metrics": body.Metrics}
					writeOperation(t, w, "operation-id", "cobra-test", command, "queued")
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "api")
				t.Setenv("DD_APP_KEY", "app")
				args := []string{command, yamlPath, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}
				if command != "validate" {
					args = append(args, "--no-poll")
				}
				var stdout, stderr bytes.Buffer
				if code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr); code != 0 {
					t.Fatalf("exit %d, stdout: %s stderr: %s", code, &stdout, &stderr)
				}
				if command == "validate" {
					if received != nil {
						t.Fatal("local validation called the API")
					}
					return
				}
				for key, want := range map[string]bool{"warehouse_metric_sources": tc.wantSource, "metrics": tc.wantMetric} {
					objects := received[key]
					if len(objects) != 1 {
						t.Fatalf("expected one %s object, got %v", key, objects)
					}
					value, present := objects[0]["upgrade_by_name"]
					if want {
						if string(value) != "true" {
							t.Errorf("%s upgrade_by_name = %s, want true", key, value)
						}
					} else if present {
						t.Errorf("%s should omit upgrade_by_name when false or unset, got %s", key, value)
					}
				}
			})
		}
	}
}
