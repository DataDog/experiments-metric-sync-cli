// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
)

func membersChange(before, after string) map[string]api.FieldChange {
	return map[string]api.FieldChange{"metrics": {Before: json.RawMessage(before), After: json.RawMessage(after)}}
}

func TestPrintResultCollectionDiffs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	a := `{"metric_sync_tag":"checkout","metric_sync_id":"a"}`
	b := `{"metric_sync_tag":"growth","metric_sync_id":"b"}`
	c := `{"metric_sync_tag":"checkout","metric_sync_id":"c"}`
	result := api.Result{
		Status: "planned",
		Plan:   true,
		Created: api.DiffBucket{MetricCollections: []api.DiffEntry{{
			SyncID: "new", ID: "c-new", Changes: membersChange(`null`, "["+a+"]"),
		}}},
		Updated: api.DiffBucket{MetricCollections: []api.DiffEntry{
			{SyncID: "reordered", ID: "c-1", Changes: membersChange("["+a+","+b+"]", "["+b+","+a+"]")},
			{SyncID: "edited", ID: "c-2", Changes: membersChange("["+a+","+b+"]", "["+a+","+c+"]")},
			{SyncID: "unknown_shape", ID: "c-3", Changes: membersChange(`["x"]`, `[{"id":"y"}]`)},
		}},
		Upgraded: api.DiffBucket{MetricCollections: []api.DiffEntry{{SyncID: "adopted", ID: "c-4"}}},
		Deleted:  api.DiffBucket{MetricCollections: []api.DiffEntry{{SyncID: "gone", ID: "c-5"}, {SyncID: "also_gone", ID: "c-6"}}},
	}
	var out bytes.Buffer
	if err := PrintResult(&out, &result); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Created    1",
		"Updated    3",
		"Deleted    2",
		"Create collection new (ID: c-new)",
		"      before: (none)\n      after: [checkout/a]\n      added: checkout/a\n",
		"Update collection reordered (ID: c-1)",
		"      before: [checkout/a, growth/b]\n      after: [growth/b, checkout/a]\n      order changed\n",
		"      added: checkout/c\n      removed: growth/b\n",
		`metrics: ["x"] -> [{"id":"y"}]`,
		"Manage with Metric Sync collection adopted (ID: c-4)",
		"Member metrics are not adopted",
		"Delete collection gone (ID: c-5)",
		"Delete collection also_gone (ID: c-6)",
	} {
		assertContains(t, got, want)
	}
	if strings.Contains(got, "No changes") || strings.Contains(got, "Warning") {
		t.Fatal(got)
	}
}

func TestPrintResultCollectionErrors(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	collectionID := "checkout_core"
	metricID := "signups"
	result := api.Result{
		Status: "failed",
		Blocked: api.DiffBucket{Metrics: []api.DiffEntry{{
			SyncID: "signups", ID: "m-1", Reason: "referenced by collection checkout_core in sync tag checkout",
		}}},
		Errors: []api.Issue{
			{ObjectType: "metric_collection", SyncID: &collectionID, Path: "metric_collections[0].metrics[1]", Message: "metric growth/signups is not synced", Remediation: "sync the metric under tag growth first"},
			{ObjectType: "metric", SyncID: &metricID, Path: "metrics", Message: "cannot delete metric signups: referenced by collection checkout_core (sync tag checkout)", Remediation: "remove the reference from checkout_core and sync tag checkout first"},
			{ObjectType: "metric_collection", Message: "no sync id"},
		},
	}
	var out bytes.Buffer
	if err := PrintResult(&out, &result); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Blocked metric signups (ID: m-1)",
		"Reason: referenced by collection checkout_core in sync tag checkout",
		"Error: metric_collection checkout_core metric_collections[0].metrics[1]: metric growth/signups is not synced",
		"Correction: sync the metric under tag growth first",
		"Error: metric signups metrics: cannot delete metric signups",
		"Correction: remove the reference from checkout_core and sync tag checkout first",
		"Error: metric_collection: no sync id",
	} {
		assertContains(t, got, want)
	}
}

func TestPrintResultCollectionMembersIncludeUnmanagedMetrics(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	result := api.Result{Status: "planned", Plan: true, Updated: api.DiffBucket{MetricCollections: []api.DiffEntry{{
		SyncID: "core", ID: "c-1",
		Changes: membersChange(
			`[{"metric_id":"m-1"},{"metric_sync_tag":"growth","metric_sync_id":"b"}]`,
			`[{"metric_sync_tag":"growth","metric_sync_id":"b"},{"metric_sync_tag":"checkout","metric_sync_id":"a"}]`,
		),
	}}}}
	var out bytes.Buffer
	if err := PrintResult(&out, &result); err != nil {
		t.Fatal(err)
	}
	assertContains(t, out.String(), "      before: [unmanaged metric m-1, growth/b]\n      after: [growth/b, checkout/a]\n      added: checkout/a\n      removed: unmanaged metric m-1\n")
}
