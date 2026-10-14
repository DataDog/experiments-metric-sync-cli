// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
)

func TestTerminalResultExitAndDetails(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		result       api.Result
		unavailable  bool
		wantCode     int
		want         string
	}{
		{name: "blocked", status: "planned", result: api.Result{Status: "planned", Blocked: api.DiffBucket{Metrics: []api.DiffEntry{{SyncID: "metric", Reason: "used by experiment"}}}}, wantCode: 1, want: "used by experiment"},
		{name: "failure with error", status: "failed", result: api.Result{Status: "failed", Errors: []api.Issue{{Message: "missing external source"}}}, wantCode: 1, want: "missing external source"},
		{name: "failure without errors", status: "failed", result: api.Result{Status: "failed"}, wantCode: 1, want: "server returned no error details"},
		{name: "unavailable failure details", status: "failed", unavailable: true, wantCode: 1, want: "operation-id has status failed"},
		{name: "result reports failure", status: "success", result: api.Result{Status: "failed"}, wantCode: 1, want: "server returned no error details"},
		{name: "warnings", status: "success", result: api.Result{Status: "success", Warnings: []string{"check this warning"}}, want: "check this warning"},
	} {
		for _, command := range []string{"plan", "execute", "result"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				yamlPath := writeValidYAMLWithWarehouseConnection(t, "connection")
				resultReads := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost:
						writeOperation(t, w, "operation-id", "test", command, "queued")
					case strings.HasSuffix(r.URL.Path, "/result"):
						resultReads++
						if tc.unavailable {
							w.WriteHeader(500)
							return
						}
						result := tc.result
						result.MetricSyncID = "operation-id"
						result.Plan = command == "plan"
						if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "operation-id", "attributes": result}}); err != nil {
							t.Error(err)
						}
					default:
						writeOperation(t, w, "operation-id", "test", command, tc.status)
					}
				}))
				defer server.Close()
				t.Setenv("DD_API_KEY", "api")
				t.Setenv("DD_APP_KEY", "app")
				positional := yamlPath
				if command == "result" {
					positional = "operation-id"
				}
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, positional, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				output := stdout.String() + stderr.String()
				if code != tc.wantCode || resultReads != 1 {
					t.Fatalf("exit=%d result reads=%d: %s", code, resultReads, output)
				}
				if !(tc.unavailable && command == "result") && !strings.Contains(output, tc.want) {
					t.Fatalf("missing %q: %s", tc.want, output)
				}
			})
		}
	}
}
