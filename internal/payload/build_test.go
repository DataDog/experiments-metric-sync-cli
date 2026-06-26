package payload

import (
	"testing"

	"github.com/DataDog/datadog-experiment-metric-sync-cli/internal/model"
)

func TestBuildMergesFiles(t *testing.T) {
	files := []model.FileConfig{
		{
			Path: "sources.yaml",
			Config: model.SyncConfig{
				SchemaVersion:         1,
				SyncTag:               "checkout",
				WarehouseConnectionID: "warehouse",
				Options:               model.Options{UpgradeMode: "by_id"},
				WarehouseMetricSources: []model.WarehouseMetricSource{
					{SyncID: "source"},
				},
			},
		},
		{
			Path: "metrics.yaml",
			Config: model.SyncConfig{
				SchemaVersion: 1,
				SyncTag:       "checkout",
				Options:       model.Options{UpgradeMode: "by_id"},
				Metrics: []model.Metric{
					{SyncID: "metric"},
				},
			},
		},
	}

	got, options, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncTag != "checkout" || len(got.WarehouseMetricSources) != 1 || len(got.Metrics) != 1 {
		t.Fatalf("unexpected merged payload: %#v", got)
	}
	if options.UpgradeMode != "by_id" {
		t.Fatalf("got upgrade mode %q", options.UpgradeMode)
	}
}

func TestBuildRejectsMismatchedSyncTag(t *testing.T) {
	_, _, err := Build([]model.FileConfig{
		{Path: "one.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "one", Options: model.Options{UpgradeMode: "none"}}},
		{Path: "two.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "two", Options: model.Options{UpgradeMode: "none"}}},
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}
