// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Operation struct {
	MetricSyncID       string  `json:"metric_sync_id"`
	SyncTag            string  `json:"sync_tag"`
	OperationType      string  `json:"operation_type"`
	Status             string  `json:"status"`
	StatusDetail       *string `json:"status_detail,omitempty"`
	PayloadHash        *string `json:"payload_hash,omitempty"`
	TemporalWorkflowID *string `json:"temporal_workflow_id,omitempty"`
	TemporalRunID      *string `json:"temporal_run_id,omitempty"`
	CreatedAt          string  `json:"created_at,omitempty"`
	StartedAt          *string `json:"started_at,omitempty"`
	CompletedAt        *string `json:"completed_at,omitempty"`
}

type Result struct {
	MetricSyncID string     `json:"metric_sync_id"`
	SyncTag      string     `json:"sync_tag"`
	Status       string     `json:"status"`
	Plan         bool       `json:"plan"`
	PayloadHash  string     `json:"payload_hash,omitempty"`
	Created      DiffBucket `json:"created"`
	Updated      DiffBucket `json:"updated"`
	Deleted      DiffBucket `json:"deleted"`
	Upgraded     DiffBucket `json:"upgraded"`
	Blocked      DiffBucket `json:"blocked"`
	Warnings     []string   `json:"warnings,omitempty"`
	Errors       []Issue    `json:"errors,omitempty"`
}

type DiffBucket struct {
	WarehouseMetricSources []DiffEntry `json:"warehouse_metric_sources"`
	Metrics                []DiffEntry `json:"metrics"`
}

type DiffEntry struct {
	SyncID                string                 `json:"sync_id"`
	ID                    string                 `json:"id"`
	Changes               map[string]FieldChange `json:"changes,omitempty"`
	Reason                string                 `json:"reason,omitempty"`
	AffectedExperimentIDs []string               `json:"affected_experiment_ids,omitempty"`
}

type FieldChange struct {
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

type Issue struct {
	Path           string          `json:"path"`
	ObjectType     string          `json:"object_type"`
	SyncID         *string         `json:"sync_id,omitempty"`
	Message        string          `json:"message"`
	Remediation    string          `json:"remediation,omitempty"`
	ExistingObject json.RawMessage `json:"existing_object,omitempty"`
}

type APIError struct {
	StatusCode int
	Errors     []ErrorItem
}

type ErrorItem struct {
	Status string `json:"status,omitempty"`
	Title  string `json:"title,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("Datadog API returned HTTP %d", e.StatusCode)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		text := item.Title
		if item.Detail != "" {
			if text != "" {
				text += ": "
			}
			text += item.Detail
		}
		if text == "" {
			text = item.Status
		}
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Datadog API returned HTTP %d", e.StatusCode)
	}
	return strings.Join(parts, "; ")
}

type ResultNotReadyError struct {
	RetryAfter string
	Message    string
}

func (e *ResultNotReadyError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "metric sync result is not ready"
}

func IsTerminalStatus(status string) bool {
	switch status {
	case "planned", "success", "failed", "skipped":
		return true
	default:
		return false
	}
}

func IsFailureStatus(status string) bool {
	switch status {
	case "failed":
		return true
	default:
		return false
	}
}

func CountBucket(bucket DiffBucket) int {
	return len(bucket.WarehouseMetricSources) + len(bucket.Metrics)
}
