// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api

import (
	"encoding/json"
	"testing"
)

func TestSkippedIsTerminalButNotFailure(t *testing.T) {
	if !IsTerminalStatus("skipped") {
		t.Fatal("skipped should be terminal")
	}
	if IsFailureStatus("skipped") {
		t.Fatal("skipped should be treated as no-op success, not failure")
	}
}

func TestFailedIsFailure(t *testing.T) {
	if !IsTerminalStatus("failed") {
		t.Fatal("failed should be terminal")
	}
	if !IsFailureStatus("failed") {
		t.Fatal("failed should be treated as failure")
	}
}

func TestResultDecodesMetricCollectionBuckets(t *testing.T) {
	var result Result
	if err := json.Unmarshal([]byte(`{
		"status": "planned",
		"created": {"warehouse_metric_sources": [], "metrics": [], "metric_collections": [{"sync_id": "core", "id": "c1"}]},
		"deleted": {"warehouse_metric_sources": [], "metrics": [], "metric_collections": [{"sync_id": "old", "id": "c2"}, {"sync_id": "older", "id": "c3"}]}
	}`), &result); err != nil {
		t.Fatal(err)
	}
	if CountBucket(result.Created) != 1 || CountBucket(result.Deleted) != 2 || result.Created.MetricCollections[0].SyncID != "core" {
		t.Fatalf("unexpected result: %+v", result)
	}

	var withoutKey Result
	if err := json.Unmarshal([]byte(`{"status": "planned", "created": {"warehouse_metric_sources": [], "metrics": []}}`), &withoutKey); err != nil {
		t.Fatal(err)
	}
	if CountBucket(withoutKey.Created) != 0 {
		t.Fatalf("expected an empty bucket, got %+v", withoutKey.Created)
	}
}
