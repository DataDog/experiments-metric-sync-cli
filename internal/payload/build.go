// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package payload

import (
	"fmt"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

type SubmitOptions struct {
	IsCertified bool   `json:"is_certified"`
	UpgradeMode string `json:"upgrade_mode"`
	ForceDelete bool   `json:"force_delete"`
}

func Build(files []model.FileConfig) (model.SyncConfig, SubmitOptions, error) {
	if len(files) == 0 {
		return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("no metric sync YAML files found")
	}

	first := files[0].Config
	result := model.SyncConfig{
		SchemaVersion:          first.SchemaVersion,
		SyncTag:                first.SyncTag,
		ReferenceURL:           first.ReferenceURL,
		WarehouseConnectionID:  first.WarehouseConnectionID,
		WarehouseMetricSources: []model.WarehouseMetricSource{},
		Metrics:                []model.Metric{},
	}
	options := first.Options.WithDefaults()

	for _, file := range files {
		config := file.Config
		if config.SchemaVersion != result.SchemaVersion {
			return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("%s: schema_version does not match %d", file.Path, result.SchemaVersion)
		}
		if config.SyncTag != result.SyncTag {
			return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("%s: sync_tag does not match %q", file.Path, result.SyncTag)
		}
		if result.ReferenceURL == "" {
			result.ReferenceURL = config.ReferenceURL
		} else if config.ReferenceURL != "" && config.ReferenceURL != result.ReferenceURL {
			return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("%s: reference_url does not match %q", file.Path, result.ReferenceURL)
		}
		if result.WarehouseConnectionID == "" {
			result.WarehouseConnectionID = config.WarehouseConnectionID
		} else if config.WarehouseConnectionID != "" && config.WarehouseConnectionID != result.WarehouseConnectionID {
			return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("%s: warehouse_connection_id does not match the first file", file.Path)
		}
		if config.Options.WithDefaults() != options {
			return model.SyncConfig{}, SubmitOptions{}, fmt.Errorf("%s: options must match across all files in one operation", file.Path)
		}
		result.WarehouseMetricSources = append(result.WarehouseMetricSources, config.WarehouseMetricSources...)
		result.Metrics = append(result.Metrics, config.Metrics...)
	}

	return result, SubmitOptions{
		IsCertified: options.IsCertified,
		UpgradeMode: options.UpgradeMode,
		ForceDelete: options.ForceDelete,
	}, nil
}
