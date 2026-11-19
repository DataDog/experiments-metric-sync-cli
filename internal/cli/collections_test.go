// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
)

// runPlanAgainst runs plan for path against a fake server that returns result,
// and returns the decoded request body, exit code, and combined output.
func runPlanAgainst(t *testing.T, path string, result api.Result) (map[string]json.RawMessage, int, string) {
	t.Helper()
	var body map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/unstable/ffe/warehouse-connections":
			writeWarehouseConnections(t, w, "connection")
		case r.Method == http.MethodPost:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Error(err)
			}
			writeOperation(t, w, "operation-id", "checkout", "plan", "queued")
		case strings.HasSuffix(r.URL.Path, "/result"):
			result.MetricSyncID = "operation-id"
			result.Plan = true
			if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "operation-id", "attributes": result}}); err != nil {
				t.Error(err)
			}
		default:
			writeOperation(t, w, "operation-id", "checkout", "plan", result.Status)
		}
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "api")
	t.Setenv("DD_APP_KEY", "app")
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{"plan", path, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
	return body, code, stdout.String() + stderr.String()
}

func TestPlanSendsMetricCollectionsAndPrintsDiffs(t *testing.T) {
	result := api.Result{Status: "planned", Created: api.DiffBucket{MetricCollections: []api.DiffEntry{{
		SyncID: "checkout_core", ID: "collection-id",
		Changes: map[string]api.FieldChange{"metrics": {
			Before: json.RawMessage(`null`),
			After:  json.RawMessage(`[{"metric_sync_tag":"checkout","metric_sync_id":"checkout_revenue"},{"metric_sync_tag":"growth","metric_sync_id":"signups"}]`),
		}},
	}}}}
	body, code, output := runPlanAgainst(t, "../../examples/collections.yaml", result)
	if code != 0 {
		t.Fatalf("exit=%d: %s", code, output)
	}
	var collections []map[string]json.RawMessage
	if err := json.Unmarshal(body["metric_collections"], &collections); err != nil || len(collections) != 1 {
		t.Fatalf("unexpected metric_collections %s: %v", body["metric_collections"], err)
	}
	collection := collections[0]
	if string(collection["is_guardrail"]) != "false" || string(collection["sync_id"]) != `"checkout_core"` {
		t.Fatalf("unexpected collection: %s", body["metric_collections"])
	}
	if _, ok := collection["existing_id"]; ok {
		t.Fatalf("unexpected existing_id: %s", body["metric_collections"])
	}
	wantMembers := `[{"metric_sync_id":"checkout_revenue"},{"metric_sync_id":"checkout_count"},{"metric_sync_tag":"growth","metric_sync_id":"signups"}]`
	if string(collection["metrics"]) != wantMembers {
		t.Fatalf("metrics = %s, want %s", collection["metrics"], wantMembers)
	}
	for _, want := range []string{"Create collection checkout_core (ID: collection-id)", "after: [checkout/checkout_revenue, growth/signups]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q: %s", want, output)
		}
	}
}

func TestPlanWithEmptyCollectionBucketsShowsNoChanges(t *testing.T) {
	empty := api.DiffBucket{WarehouseMetricSources: []api.DiffEntry{}, Metrics: []api.DiffEntry{}, MetricCollections: []api.DiffEntry{}}
	result := api.Result{Status: "planned", Created: empty, Updated: empty, Deleted: empty, Upgraded: empty, Blocked: empty}
	_, code, output := runPlanAgainst(t, "../../examples/collections.yaml", result)
	if code != 0 || !strings.Contains(output, "No changes") {
		t.Fatalf("exit=%d: %s", code, output)
	}
}

func TestPlanWithoutCollectionsShowsServerDeletions(t *testing.T) {
	result := api.Result{Status: "planned", Deleted: api.DiffBucket{MetricCollections: []api.DiffEntry{
		{SyncID: "checkout_core", ID: "c-1"},
		{SyncID: "checkout_extra", ID: "c-2"},
	}}}
	body, code, output := runPlanAgainst(t, "../../examples/minimal.yaml", result)
	if code != 0 {
		t.Fatalf("exit=%d: %s", code, output)
	}
	if _, ok := body["metric_collections"]; ok {
		t.Fatalf("expected no metric_collections key, got %s", body["metric_collections"])
	}
	for _, want := range []string{"Deleted    2", "Delete collection checkout_core (ID: c-1)", "Delete collection checkout_extra (ID: c-2)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "No changes") {
		t.Fatal(output)
	}
}

func TestPlanReportsCollectionMemberAndDependencyErrors(t *testing.T) {
	collectionID := "checkout_core"
	metricID := "checkout_count"
	for _, tc := range []struct {
		name   string
		result api.Result
		want   []string
	}{
		{
			name: "member error",
			result: api.Result{Status: "failed", Errors: []api.Issue{{
				ObjectType: "metric_collection", SyncID: &collectionID, Path: "metric_collections[0].metrics[2]",
				Message: "metric growth/signups is not synced", Remediation: "sync metric signups under tag growth first",
			}}},
			want: []string{"Error: metric_collection checkout_core metric_collections[0].metrics[2]: metric growth/signups is not synced", "Correction: sync metric signups under tag growth first"},
		},
		{
			name: "dependency conflict",
			result: api.Result{
				Status:  "planned",
				Blocked: api.DiffBucket{Metrics: []api.DiffEntry{{SyncID: metricID, ID: "m-1", Reason: "referenced by collection growth_core in sync tag growth"}}},
				Errors: []api.Issue{{
					ObjectType: "metric", SyncID: &metricID, Path: "metrics",
					Message: "cannot delete metric checkout_count: referenced by collection growth_core (sync tag growth)", Remediation: "remove the reference from growth_core and sync tag growth first",
				}},
			},
			want: []string{"Blocked metric checkout_count (ID: m-1)", "Reason: referenced by collection growth_core in sync tag growth", "Correction: remove the reference from growth_core and sync tag growth first"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, code, output := runPlanAgainst(t, "../../examples/collections.yaml", tc.result)
			if code != 1 {
				t.Fatalf("exit=%d: %s", code, output)
			}
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Fatalf("missing %q: %s", want, output)
				}
			}
		})
	}
}

func TestValidateCollectionExamples(t *testing.T) {
	for _, example := range []string{"../../examples/collections.yaml", "../../examples/collections-adoption.yaml"} {
		var stdout, stderr bytes.Buffer
		code := runWithIO([]string{"validate", example, "--log-file", filepath.Join(t.TempDir(), "metric-sync.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit=%d: %s%s", example, code, stdout.String(), stderr.String())
		}
	}
}
