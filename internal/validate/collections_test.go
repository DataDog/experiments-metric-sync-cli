// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func collectionConfig(collections ...model.MetricCollection) model.SyncConfig {
	config := thresholdConfig(model.Metric{
		SyncID:     "revenue",
		Name:       "Revenue",
		MetricType: "simple",
		SimpleMetricAggregation: &model.SimpleMetricAggregation{
			Operation: "sum",
			Measure:   model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
		},
	})
	config.MetricCollections = collections
	return config
}

func members(ids ...string) []model.MetricCollectionMember {
	result := make([]model.MetricCollectionMember, len(ids))
	for i, id := range ids {
		result[i] = model.MetricCollectionMember{MetricSyncID: id}
	}
	return result
}

func TestValidateMetricCollections(t *testing.T) {
	collection := func(name string, metrics []model.MetricCollectionMember) model.MetricCollection {
		return model.MetricCollection{SyncID: "core", Name: name, Metrics: metrics}
	}
	tooMany := make([]model.MetricCollectionMember, 101)
	for i := range tooMany {
		tooMany[i] = model.MetricCollectionMember{MetricSyncTag: "growth", MetricSyncID: fmt.Sprintf("m%d", i)}
	}

	for _, tc := range []struct {
		name  string
		files []model.FileConfig
		path  string
		want  string
	}{
		{name: "valid with cross-tag member", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Checkout core", []model.MetricCollectionMember{{MetricSyncID: "revenue"}, {MetricSyncTag: "growth", MetricSyncID: "signups"}}))}}},
		{name: "empty members", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", nil))}}},
		{name: "name of three after trimming", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("  abc  ", nil))}}},
		{name: "name of forty", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection(" "+strings.Repeat("時", 40)+" ", nil))}}},
		{name: "name of two after trimming", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("  ab  ", nil))}}, path: "metric_collections[0].name", want: "3 to 40"},
		{name: "name of forty-one", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection(strings.Repeat("a", 41), nil))}}, path: "metric_collections[0].name", want: "3 to 40"},
		{name: "missing sync_id", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(model.MetricCollection{Name: "Core"})}}, path: "metric_collections[0].sync_id", want: "is required"},
		{name: "hundred members", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", tooMany[:100]))}}},
		{name: "hundred and one members", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", tooMany))}}, path: "metric_collections[0].metrics", want: "at most 100"},
		{name: "duplicate member with omitted and explicit current tag", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", []model.MetricCollectionMember{{MetricSyncID: "revenue"}, {MetricSyncTag: "checkout", MetricSyncID: "revenue"}}))}}, path: "metric_collections[0].metrics[1]", want: "duplicates metric_collections[0].metrics[0]"},
		{name: "same id under other tag is not a duplicate", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", []model.MetricCollectionMember{{MetricSyncID: "revenue"}, {MetricSyncTag: "growth", MetricSyncID: "revenue"}}))}}},
		{name: "undefined same-tag member", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", members("missing")))}}, path: "metric_collections[0].metrics[0].metric_sync_id", want: "does not match a metric sync_id"},
		{name: "member existing_id", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", []model.MetricCollectionMember{{ExistingID: "00000000-0000-0000-0000-000000000000"}}))}}, path: "metric_collections[0].metrics[0].existing_id", want: "collection members must be synced metrics; reference them by metric_sync_id"},
		{name: "invalid member tag", files: []model.FileConfig{{Path: "a.yaml", Config: collectionConfig(collection("Core", []model.MetricCollectionMember{{MetricSyncTag: "-bad", MetricSyncID: "x"}}))}}, path: "metric_collections[0].metrics[0].metric_sync_tag", want: "must start with"},
		{
			name: "duplicate sync_id across files",
			files: []model.FileConfig{
				{Path: "a.yaml", Config: collectionConfig(model.MetricCollection{SyncID: "core", Name: "First"})},
				{Path: "b.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout", MetricCollections: []model.MetricCollection{{SyncID: "core", Name: "Second"}}}},
			},
			path: "metric_collections[0].sync_id", want: "duplicates collection sync_id from a.yaml",
		},
		{
			name: "duplicate trimmed name across files",
			files: []model.FileConfig{
				{Path: "a.yaml", Config: collectionConfig(model.MetricCollection{SyncID: "one", Name: "Core"})},
				{Path: "b.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout", MetricCollections: []model.MetricCollection{{SyncID: "two", Name: " Core "}}}},
			},
			path: "metric_collections[0].name", want: "duplicates collection name from a.yaml",
		},
		{
			name: "member defined in another file",
			files: []model.FileConfig{
				{Path: "a.yaml", Config: collectionConfig()},
				{Path: "b.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout", MetricCollections: []model.MetricCollection{{SyncID: "two", Name: "Core", Metrics: members("revenue")}}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := ValidateFiles(tc.files)
			if tc.want == "" {
				if len(issues) != 0 {
					t.Fatalf("expected no issues, got %v", issues)
				}
				return
			}
			if len(issues) != 1 || issues[0].Path != tc.path || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("expected one issue at %s containing %q, got %v", tc.path, tc.want, issues)
			}
		})
	}
}
