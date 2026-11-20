// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestPropertyBreakouts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*model.SyncConfig)
		want   string
	}{
		{"valid", func(*model.SyncConfig) {}, ""},
		{"clear", func(c *model.SyncConfig) { c.Metrics[0].PropertyBreakouts = &[]model.PropertyBreakout{} }, ""},
		{"missing property", func(c *model.SyncConfig) { (*c.Metrics[0].PropertyBreakouts)[0].Property = nil }, ".property: is required"},
		{"unknown property", func(c *model.SyncConfig) { (*c.Metrics[0].PropertyBreakouts)[0].Property.PropertySyncID = "missing" }, "does not match a property"},
		{"empty values", func(c *model.SyncConfig) { (*c.Metrics[0].PropertyBreakouts)[0].Values = nil }, "at least one value"},
		{"duplicates", func(c *model.SyncConfig) { (*c.Metrics[0].PropertyBreakouts)[0].Values = []string{"US", "US"} }, "distinct nonempty"},
		{"blank", func(c *model.SyncConfig) { (*c.Metrics[0].PropertyBreakouts)[0].Values = []string{" "} }, "distinct nonempty"},
		{"duplicate property", func(c *model.SyncConfig) {
			*c.Metrics[0].PropertyBreakouts = append(*c.Metrics[0].PropertyBreakouts, (*c.Metrics[0].PropertyBreakouts)[0])
		}, "duplicate breakout property"},
		{"wrong source", func(c *model.SyncConfig) {
			(*c.Metrics[0].PropertyBreakouts)[0].Property.WarehouseMetricSourceSyncID = "other"
		}, "every aggregation source"},
		{"cross source ratio", func(c *model.SyncConfig) {
			a := *c.Metrics[0].SimpleMetricAggregation
			b := a
			b.Measure.WarehouseMetricSourceSyncID = "other"
			b.Measure.WarehouseMetricSourceSyncTag = "external"
			c.Metrics[0].SimpleMetricAggregation = nil
			c.Metrics[0].MetricType = "ratio"
			c.Metrics[0].RatioMetricAggregation = &model.RatioMetricAggregation{NumeratorAggregation: a, DenominatorAggregation: b}
		}, "cross-source ratio"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := measureRefConfig(model.MeasureRef{WarehouseMetricSourceSyncID: "src", MeasureSyncID: "m"}, "sum")
			c.WarehouseMetricSources[0].Properties = []model.Property{{SyncID: "country", Name: "Country", ColumnName: "country", ColumnType: "STRING"}}
			c.Metrics[0].PropertyBreakouts = &[]model.PropertyBreakout{{Property: &model.PropertyRef{WarehouseMetricSourceSyncID: "src", PropertySyncID: "country"}, Values: []string{"US", "CA"}}}
			tc.mutate(&c)
			issues := ValidateFiles([]model.FileConfig{{Path: "test.yaml", Config: c}})
			if tc.want == "" {
				if len(issues) > 0 {
					t.Fatalf("unexpected issues: %v", issues)
				}
				return
			}
			for _, issue := range issues {
				if strings.Contains(issue.Error(), tc.want) {
					return
				}
			}
			t.Fatalf("wanted %q, got %v", tc.want, issues)
		})
	}
}
