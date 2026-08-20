// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package payload

import (
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
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
	if !options.IsCertified {
		t.Fatal("expected omitted is_certified to default to true")
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

func TestBuildAllowsExplicitUncertified(t *testing.T) {
	isCertified := false
	_, options, err := Build([]model.FileConfig{{
		Path: "metric.yaml",
		Config: model.SyncConfig{
			SchemaVersion: 1,
			SyncTag:       "checkout",
			Options:       model.Options{IsCertified: &isCertified},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if options.IsCertified {
		t.Fatal("expected explicit is_certified=false to be preserved")
	}
}

func TestBuildRejectsMismatchedCertifiedOptions(t *testing.T) {
	isCertified := false
	_, _, err := Build([]model.FileConfig{
		{Path: "one.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout"}},
		{Path: "two.yaml", Config: model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout", Options: model.Options{IsCertified: &isCertified}}},
	})
	if err == nil {
		t.Fatal("expected options mismatch error")
	}
}

func TestBuildPreservesThresholdFields(t *testing.T) {
	breach := 10.0
	timeframe := 7.0
	files := []model.FileConfig{{
		Path: "threshold.yaml",
		Config: model.SyncConfig{
			SchemaVersion: 1,
			SyncTag:       "checkout",
			Metrics: []model.Metric{{
				SyncID:     "metric",
				Name:       "metric",
				MetricType: "simple",
				SimpleMetricAggregation: &model.SimpleMetricAggregation{
					Operation:                   "threshold",
					Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
					ThresholdAggregationType:    "count",
					ThresholdComparisonOperator: "gt",
					ThresholdBreachValue:        &breach,
					ThresholdTimeframeValue:     &timeframe,
					ThresholdTimeframeDimension: "days",
				},
			}},
		},
	}}

	got, _, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	agg := got.Metrics[0].SimpleMetricAggregation
	if agg.Operation != "threshold" {
		t.Fatalf("operation = %q, want threshold", agg.Operation)
	}
	if agg.ThresholdAggregationType != "count" || agg.ThresholdComparisonOperator != "gt" || agg.ThresholdTimeframeDimension != "days" {
		t.Fatalf("threshold fields not preserved: %#v", agg)
	}
	if agg.ThresholdBreachValue == nil || *agg.ThresholdBreachValue != 10 {
		t.Fatalf("threshold_breach_value = %v, want 10", agg.ThresholdBreachValue)
	}
	if agg.ThresholdTimeframeValue == nil || *agg.ThresholdTimeframeValue != 7 {
		t.Fatalf("threshold_timeframe_value = %v, want 7", agg.ThresholdTimeframeValue)
	}
}
