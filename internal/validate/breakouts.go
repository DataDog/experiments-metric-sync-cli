// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"fmt"
	"strings"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func validatePropertyBreakouts(add func(string, string), path string, metric model.Metric, syncTag string, sources map[string]model.WarehouseMetricSource) {
	if metric.PropertyBreakouts == nil {
		return
	}
	var measures []model.MeasureRef
	if a := metric.SimpleMetricAggregation; a != nil {
		measures = append(measures, a.Measure)
	}
	if a := metric.PercentileMetricAggregation; a != nil {
		measures = append(measures, a.Measure)
	}
	if a := metric.RatioMetricAggregation; a != nil {
		measures = append(measures, a.NumeratorAggregation.Measure, a.DenominatorAggregation.Measure)
	}
	seen := map[string]bool{}
	for i, breakout := range *metric.PropertyBreakouts {
		p := fmt.Sprintf("%s[%d]", path, i)
		if breakout.Property == nil {
			add(p+".property", "is required")
			continue
		}
		ref := breakout.Property
		requireSyncID(add, p+".property.warehouse_metric_source_sync_id", ref.WarehouseMetricSourceSyncID)
		requireSyncID(add, p+".property.property_sync_id", ref.PropertySyncID)
		if ref.WarehouseMetricPropertyID != "" {
			add(p+".property.warehouse_metric_property_id", "is not supported; use stable source and property sync IDs")
		}
		tag := ref.WarehouseMetricSourceSyncTag
		if tag == "" {
			tag = syncTag
		} else {
			requireSyncID(add, p+".property.warehouse_metric_source_sync_tag", tag)
		}
		key := tag + "/" + ref.WarehouseMetricSourceSyncID + "/" + ref.PropertySyncID
		if seen[key] {
			add(p+".property", "duplicate breakout property")
		}
		seen[key] = true
		for _, measure := range measures {
			measureTag := measure.WarehouseMetricSourceSyncTag
			if measureTag == "" {
				measureTag = syncTag
			}
			if tag != measureTag || ref.WarehouseMetricSourceSyncID != measure.WarehouseMetricSourceSyncID {
				add(p+".property", "must belong to every aggregation source; cross-source ratio breakouts are not supported")
				break
			}
		}
		source, local := sources[ref.WarehouseMetricSourceSyncID]
		if ref.WarehouseMetricSourceSyncTag == "" || (tag == syncTag && local) {
			if !local {
				add(p+".property.warehouse_metric_source_sync_id", "does not match a source in this operation; include the source or specify its sync tag")
			} else {
				found := false
				for _, property := range source.Properties {
					found = found || property.SyncID == ref.PropertySyncID
				}
				if !found {
					add(p+".property.property_sync_id", "does not match a property on the referenced source")
				}
			}
		}
		if len(breakout.Values) == 0 {
			add(p+".values", "must contain at least one value")
		}
		values := map[string]bool{}
		for _, value := range breakout.Values {
			if strings.TrimSpace(value) == "" || values[value] {
				add(p+".values", "must contain distinct nonempty strings")
			}
			values[value] = true
		}
	}
}
