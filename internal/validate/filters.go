// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

var (
	integerFilterOperandPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)
	floatFilterOperandPattern   = regexp.MustCompile(`^[+-]?(?:(?:[0-9]+(?:\.[0-9]*)?)|(?:\.[0-9]+))(?:[eE][+-]?[0-9]+)?$`)
)

func validateFilters(add func(string, string), path string, groups model.PropertyFilters, aggregation model.MeasureRef, allowMeasure bool, syncTag string, sources map[string]model.WarehouseMetricSource) {
	for i, group := range groups {
		if len(group) == 0 {
			add(fmt.Sprintf("%s[%d]", path, i), "must contain at least one filter")
		}
		for j, filter := range group {
			p := fmt.Sprintf("%s[%d][%d]", path, i, j)
			if (filter.Property == nil) == (filter.Measure == nil) {
				add(p, "must set exactly one of property or measure")
				continue
			}
			if filter.Property != nil {
				ref := filter.Property
				require(add, p+".property.warehouse_metric_source_sync_id", ref.WarehouseMetricSourceSyncID)
				require(add, p+".property.property_sync_id", ref.PropertySyncID)
				if ref.WarehouseMetricPropertyID != "" {
					add(p+".property.warehouse_metric_property_id", "is not supported by the sync API; use source and property sync IDs")
				}
				validateFilterSource(add, p+".property", ref.WarehouseMetricSourceSyncID, ref.WarehouseMetricSourceSyncTag, aggregation, syncTag)
				source, local := sources[ref.WarehouseMetricSourceSyncID]
				if ref.WarehouseMetricSourceSyncTag == "" || (ref.WarehouseMetricSourceSyncTag == syncTag && local) {
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
				switch filter.Operation {
				case "IS", "IS_NOT":
					if len(filter.Values) == 0 {
						add(p+".values", "must contain at least one value")
					}
				case "IS_NULL", "IS_NOT_NULL":
					if !allowMeasure {
						add(p+".operation", "percentile property filters support only IS and IS_NOT")
					}
					if len(filter.Values) != 0 {
						add(p+".values", "must be empty for a null operation")
					}
				default:
					add(p+".operation", "must be IS, IS_NOT, IS_NULL, or IS_NOT_NULL for a property filter")
				}
			} else {
				ref := *filter.Measure
				if !allowMeasure {
					add(p+".measure", "measure filters are not supported for percentile metrics")
				}
				validMeasure := validateMeasureRef(add, p+".measure", ref, syncTag, sources)
				if ref.MeasureSyncID == "" {
					add(p+".measure.measure_sync_id", "must select a user-defined numeric measure")
				}
				validateFilterSource(add, p+".measure", ref.WarehouseMetricSourceSyncID, ref.WarehouseMetricSourceSyncTag, aggregation, syncTag)
				count := -1
				switch filter.Operation {
				case "=", "!=", ">", ">=", "<", "<=":
					count = 1
				case "BETWEEN":
					count = 2
				case "IS_NULL", "IS_NOT_NULL":
					count = 0
				default:
					add(p+".operation", "must be =, !=, >, >=, <, <=, BETWEEN, IS_NULL, or IS_NOT_NULL for a measure filter")
				}
				if count >= 0 && len(filter.Values) != count {
					add(p+".values", fmt.Sprintf("must contain exactly %d values for %s", count, filter.Operation))
				} else if count >= 0 && allowMeasure && validMeasure && ref.MeasureSyncID != "" {
					validateMeasureFilterOperands(add, p, filter, syncTag, sources)
				}
			}
		}
	}
}

func validateMeasureFilterOperands(add func(string, string), path string, filter model.PropertyFilter, syncTag string, sources map[string]model.WarehouseMetricSource) {
	ref := filter.Measure
	source, local := sources[ref.WarehouseMetricSourceSyncID]
	// The server resolves types for sources outside this payload.
	if !local || (ref.WarehouseMetricSourceSyncTag != "" && ref.WarehouseMetricSourceSyncTag != syncTag) {
		return
	}
	for _, measure := range source.Measures {
		if measure.SyncID != ref.MeasureSyncID {
			continue
		}
		if !oneOf(measure.ColumnType, "INTEGER", "FLOAT") {
			add(path+".measure", "filter measure must have INTEGER or FLOAT column_type")
		} else if err := validateNumericFilterValues(filter.Operation, filter.Values, measure.ColumnType); err != nil {
			add(path+".values", err.Error())
		}
		return
	}
}

// Match the backend's base-10 syntax, numeric limits, and BETWEEN ordering.
// The caller checks the operation and value count before parsing operands.
func validateNumericFilterValues(operation string, values []string, columnType string) error {
	numericValues := make([]float64, len(values))
	integerValues := make([]int64, len(values))
	for i, value := range values {
		if value == "" {
			return fmt.Errorf("measure filter value %d must be a finite base-10 numeric literal", i)
		}
		if columnType == "INTEGER" {
			if !integerFilterOperandPattern.MatchString(value) {
				return fmt.Errorf("measure filter value %d must be an integer", i)
			}
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("measure filter value %d must be a 64-bit integer", i)
			}
			integerValues[i] = parsed
		} else if !floatFilterOperandPattern.MatchString(value) {
			return fmt.Errorf("measure filter value %d must be a finite base-10 numeric literal", i)
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return fmt.Errorf("measure filter value %d must be a finite base-10 numeric literal", i)
		}
		numericValues[i] = parsed
	}
	if operation == "BETWEEN" {
		if (columnType == "INTEGER" && integerValues[0] > integerValues[1]) ||
			(columnType == "FLOAT" && numericValues[0] > numericValues[1]) {
			return fmt.Errorf("BETWEEN lower bound must be less than or equal to upper bound")
		}
	}
	return nil
}

func validateFilterSource(add func(string, string), path, sourceID, tag string, aggregation model.MeasureRef, syncTag string) {
	if tag == "" {
		tag = syncTag
	}
	aggregationTag := aggregation.WarehouseMetricSourceSyncTag
	if aggregationTag == "" {
		aggregationTag = syncTag
	}
	if sourceID != aggregation.WarehouseMetricSourceSyncID || tag != aggregationTag {
		add(path, "must reference the same source and sync tag as the aggregation measure")
	}
}
