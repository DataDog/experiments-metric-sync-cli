// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
	"github.com/DataDog/experiments-metric-sync-cli/internal/validate"
)

func PrintDiscovered(w io.Writer, files []string) error {
	if _, err := fmt.Fprintln(w, heading("Discovered files")); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(w, "  %s\n", file); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func PrintValidation(w io.Writer, issues []validate.Issue) error {
	if len(issues) == 0 {
		return renderRows(w, "Validation", 10, []row{
			{label: "Status", value: colorGreen("passed (local checks)")},
			{label: "Next step", value: "Run plan to preview changes. External references are resolved during execute."},
		})
	}
	if err := renderRows(w, "Validation", 10, []row{
		{label: "Status", value: colorRed("failed")},
		{label: "Errors", value: colorRed(strconv.Itoa(len(issues)))},
	}); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, heading("Errors")); err != nil {
		return err
	}
	for _, issue := range issues {
		if _, err := fmt.Fprintf(w, "  - %s\n", colorRed(issue.Error())); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func PrintOperation(w io.Writer, operation *api.Operation) error {
	if err := renderMetricSync(w, operation.MetricSyncID, displayOperation(operation.OperationType), operation.Status, operation.SyncTag); err != nil {
		return err
	}
	if operation.StatusDetail != nil {
		_, err := fmt.Fprintf(w, "Status detail: %s\n", *operation.StatusDetail)
		return err
	}
	return nil
}

func PrintSubmittedOperation(w io.Writer, operation *api.Operation, idempotencyKey string) error {
	return renderRows(w, "Metric sync", 16, []row{
		{label: "ID", value: operation.MetricSyncID},
		{label: "Operation", value: displayOperation(operation.OperationType)},
		{label: "Status", value: colorStatus(operation.Status)},
		{label: "Sync tag", value: operation.SyncTag},
		{label: "Idempotency key", value: idempotencyKey},
		{label: "Completion", value: "Submission accepted; completion is not confirmed."},
	})
}

func PrintResult(w io.Writer, result *api.Result) error {
	summary := resultSummary(result)
	if err := renderMetricSync(w, result.MetricSyncID, operationName(result), result.Status, result.SyncTag); err != nil {
		return err
	}
	summaryTitle := "Summary"
	if result.Plan {
		summaryTitle = "Proposed changes"
	} else if result.Status != "success" && result.Status != "skipped" {
		summaryTitle = "Proposed changes (completion is not confirmed)"
	}
	if err := renderRows(w, summaryTitle, 10, []row{
		{label: "Created", value: strconv.Itoa(summary.counts.created)},
		{label: "Updated", value: strconv.Itoa(summary.counts.updated)},
		{label: "Deleted", value: strconv.Itoa(summary.counts.deleted)},
		{label: "Upgraded", value: strconv.Itoa(summary.counts.upgraded)},
		{label: "Blocked", value: colorNonZero(summary.counts.blocked)},
		{label: "Errors", value: colorNonZero(summary.counts.errors)},
	}); err != nil {
		return err
	}
	if err := printResultDetails(w, result); err != nil {
		return err
	}
	if !summary.hasChanges() && (result.Status == "success" || result.Status == "planned" || result.Status == "skipped") {
		return renderRows(w, "No changes", 10, []row{
			{label: "Result", value: "Datadog already matches the submitted metric definitions."},
		})
	}
	return nil
}

func PrintVersion(w io.Writer, version string, commit string, date string) error {
	return renderRows(w, "Metric sync CLI", 8, []row{
		{label: "Version", value: version},
		{label: "Commit", value: commit},
		{label: "Date", value: date},
	})
}

func PrintLogWarning(w io.Writer, path string, err error) error {
	_, writeErr := fmt.Fprintf(w, "%s unable to create debug log %s: %v\n", colorYellow("warning:"), path, err)
	return writeErr
}

func PrintCommandError(w io.Writer, err error, logPath string) error {
	if err == nil {
		return nil
	}
	if _, writeErr := fmt.Fprintln(w, colorRed(err.Error())); writeErr != nil {
		return writeErr
	}
	if logPath == "" {
		return nil
	}
	_, writeErr := fmt.Fprintf(w, "debug log: %s\n", logPath)
	return writeErr
}

func PrintPollingStart(w io.Writer) error {
	_, err := fmt.Fprintln(w, heading("Polling"))
	return err
}

func PrintPollResult(w io.Writer, attempt int, operation *api.Operation, terminal bool) error {
	if operation == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "  %-10s %s\n", fmt.Sprintf("Poll %d", attempt), colorStatus(operation.Status)); err != nil {
		return err
	}
	if !terminal {
		return nil
	}
	_, err := fmt.Fprintln(w)
	return err
}

func PrintPollWait(w io.Writer, interval time.Duration, elapsed time.Duration) error {
	_, err := fmt.Fprintf(w, "  %-10s %s before next poll (elapsed %s)\n", "Waiting", formatDuration(interval), formatElapsed(elapsed))
	return err
}

type row struct {
	label string
	value string
}

func renderMetricSync(w io.Writer, id string, operation string, status string, syncTag string) error {
	return renderRows(w, "Metric sync", 10, []row{
		{label: "ID", value: id},
		{label: "Operation", value: operation},
		{label: "Status", value: colorStatus(status)},
		{label: "Sync tag", value: syncTag},
	})
}

func renderRows(w io.Writer, title string, labelWidth int, rows []row) error {
	if _, err := fmt.Fprintln(w, heading(title)); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "  %-*s %s\n", labelWidth, row.label, row.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func heading(value string) string {
	return style("1", value)
}

func colorStatus(status string) string {
	switch status {
	case "success":
		return colorGreen(status)
	case "planned":
		return colorCyan(status)
	case "queued", "running":
		return colorYellow(status)
	case "failed":
		return colorRed(status)
	default:
		return status
	}
}

func colorNonZero(value int) string {
	if value > 0 {
		return colorRed(strconv.Itoa(value))
	}
	return strconv.Itoa(value)
}

func colorGreen(value string) string {
	return style("32", value)
}

func colorCyan(value string) string {
	return style("36", value)
}

func colorYellow(value string) string {
	return style("33", value)
}

func colorRed(value string) string {
	return style("31", value)
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return duration.Round(time.Millisecond).String()
}

func formatElapsed(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return duration.Truncate(time.Second).String()
}

func style(code string, value string) string {
	if os.Getenv("NO_COLOR") != "" {
		return value
	}
	return "\033[" + code + "m" + value + "\033[0m"
}

type summary struct {
	counts summaryCounts
}

type summaryCounts struct {
	created  int
	updated  int
	deleted  int
	upgraded int
	blocked  int
	errors   int
	warnings int
}

func (s summary) hasChanges() bool {
	return s.counts.created > 0 ||
		s.counts.updated > 0 ||
		s.counts.deleted > 0 ||
		s.counts.upgraded > 0 ||
		s.counts.blocked > 0 ||
		s.counts.errors > 0 ||
		s.counts.warnings > 0
}

func resultSummary(result *api.Result) summary {
	return summary{
		counts: summaryCounts{
			created:  api.CountBucket(result.Created),
			updated:  api.CountBucket(result.Updated),
			deleted:  api.CountBucket(result.Deleted),
			upgraded: api.CountBucket(result.Upgraded),
			blocked:  api.CountBucket(result.Blocked),
			errors:   len(result.Errors),
			warnings: len(result.Warnings),
		},
	}
}

func operationName(result *api.Result) string {
	if result.Plan {
		return "plan"
	}
	return "execute"
}

func displayOperation(operation string) string {
	if operation == "sync" {
		return "execute"
	}
	return operation
}

func printResultDetails(w io.Writer, result *api.Result) error {
	for _, action := range []struct {
		name   string
		bucket api.DiffBucket
	}{
		{"Create", result.Created}, {"Update", result.Updated}, {"Delete", result.Deleted}, {"Manage with Metric Sync", result.Upgraded}, {"Blocked", result.Blocked},
	} {
		for _, objects := range []struct {
			name    string
			entries []api.DiffEntry
		}{{"source", action.bucket.WarehouseMetricSources}, {"metric", action.bucket.Metrics}} {
			for _, entry := range objects.entries {
				if _, err := fmt.Fprintf(w, "  %s %s %s (ID: %s)\n", action.name, objects.name, entry.SyncID, entry.ID); err != nil {
					return err
				}
				if action.name == "Manage with Metric Sync" {
					if _, err := fmt.Fprintln(w, "    Keep the object ID and apply the YAML settings."); err != nil {
						return err
					}
				}
				if entry.Reason != "" {
					if _, err := fmt.Fprintf(w, "    Reason: %s\n", entry.Reason); err != nil {
						return err
					}
				}
				for _, id := range entry.AffectedExperimentIDs {
					if _, err := fmt.Fprintf(w, "    Affected experiment: %s\n", id); err != nil {
						return err
					}
				}
				fields := make([]string, 0, len(entry.Changes))
				for field := range entry.Changes {
					fields = append(fields, field)
				}
				sort.Strings(fields)
				for _, field := range fields {
					change := entry.Changes[field]
					before, after := string(change.Before), string(change.After)
					if before == "" {
						before = "unknown"
					}
					if after == "" {
						after = "unknown"
					}
					if _, err := fmt.Fprintf(w, "    %s: %s -> %s\n", field, before, after); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(w, "  Warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, issue := range result.Errors {
		id := ""
		if issue.SyncID != nil {
			id = *issue.SyncID
		}
		if _, err := fmt.Fprintf(w, "  Error: %s %s %s: %s\n", issue.ObjectType, id, issue.Path, issue.Message); err != nil {
			return err
		}
		if issue.Remediation != "" {
			if _, err := fmt.Fprintf(w, "    Correction: %s\n", issue.Remediation); err != nil {
				return err
			}
		}
	}
	if api.IsFailureStatus(result.Status) && len(result.Errors) == 0 {
		_, err := fmt.Fprintf(w, "  Operation %s failed. The server returned no error details.\n", result.MetricSyncID)
		return err
	}
	return nil
}
