package output

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/metric-sync-cli/internal/api"
	"github.com/DataDog/metric-sync-cli/internal/validate"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestPrintDiscoveredText(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	err := PrintDiscovered(&out, []string{"examples/one.yaml", "examples/two.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "Discovered files\n")
	assertContains(t, got, "  examples/one.yaml\n")
	assertContains(t, got, "  examples/two.yaml\n")
	assertNoANSI(t, got)
}

func TestPrintOperationTextAlignsLabels(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	err := PrintOperation(&out, &api.Operation{
		MetricSyncID:  "06a3d7fa-532d-7000-8035-562dc7c9c0f1",
		OperationType: "sync",
		Status:        "success",
		SyncTag:       "ms-cli-test-sessions",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "Metric sync\n")
	assertContains(t, got, "  ID         06a3d7fa-532d-7000-8035-562dc7c9c0f1\n")
	assertContains(t, got, "  Operation  execute\n")
	assertContains(t, got, "  Status     success\n")
	assertContains(t, got, "  Sync tag   ms-cli-test-sessions\n")
	assertNoANSI(t, got)
}

func TestPrintResultTextIncludesMetricSyncAndSummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	err := PrintResult(&out, &api.Result{
		MetricSyncID: "06a3d7fa-532d-7000-8035-562dc7c9c0f1",
		SyncTag:      "ms-cli-test-sessions",
		Status:       "success",
		Created: api.DiffBucket{
			WarehouseMetricSources: []api.DiffEntry{{SyncID: "source"}},
			Metrics:                []api.DiffEntry{{SyncID: "metric"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "Metric sync\n")
	assertContains(t, got, "  Operation  execute\n")
	assertContains(t, got, "Summary\n")
	assertContains(t, got, "  Created    2\n")
	assertContains(t, got, "  Updated    0\n")
	assertContains(t, got, "  Deleted    0\n")
	assertContains(t, got, "  Upgraded   0\n")
	assertContains(t, got, "  Blocked    0\n")
	assertContains(t, got, "  Errors     0\n")
	assertNoANSI(t, got)
}

func TestPrintResultTextShowsNoChangesForEmptySummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	err := PrintResult(&out, &api.Result{
		MetricSyncID: "06a3d8f1-12d5-7000-9a63-577e7ac75b4f",
		SyncTag:      "ms-cli-test-sessions",
		Status:       "success",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "Summary\n")
	assertContains(t, got, "  Created    0\n")
	assertContains(t, got, "No changes\n")
	assertContains(t, got, "  Result     Datadog already matches the submitted metric definitions.\n")
	assertNoANSI(t, got)
}

func TestPrintResultTextOmitsNoChangesForNonEmptySummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	err := PrintResult(&out, &api.Result{
		MetricSyncID: "06a3d7fa-532d-7000-8035-562dc7c9c0f1",
		SyncTag:      "ms-cli-test-sessions",
		Status:       "success",
		Created: api.DiffBucket{
			Metrics: []api.DiffEntry{{SyncID: "metric"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "  Created    1\n")
	if strings.Contains(got, "No changes") {
		t.Fatalf("expected no no-changes section for non-empty summary, got %q", got)
	}
	assertNoANSI(t, got)
}

func TestPrintValidationText(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var success bytes.Buffer
	if err := PrintValidation(&success, nil); err != nil {
		t.Fatal(err)
	}
	assertContains(t, success.String(), "Validation\n")
	assertContains(t, success.String(), "  Status     passed\n")
	assertNoANSI(t, success.String())

	var failure bytes.Buffer
	err := PrintValidation(&failure, []validate.Issue{{
		File:    "metrics.yaml",
		Path:    "sync_tag",
		Message: "is required",
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := failure.String()
	assertContains(t, got, "Validation\n")
	assertContains(t, got, "  Status     failed\n")
	assertContains(t, got, "  Errors     1\n")
	assertContains(t, got, "Errors\n")
	assertContains(t, got, "  - metrics.yaml: sync_tag: is required\n")
	assertNoANSI(t, got)
}

func TestPrintPollingText(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	if err := PrintPollingStart(&out); err != nil {
		t.Fatal(err)
	}
	if err := PrintPollResult(&out, 1, &api.Operation{Status: "queued"}, false); err != nil {
		t.Fatal(err)
	}
	if err := PrintPollWait(&out, time.Second, 1200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := PrintPollResult(&out, 2, &api.Operation{Status: "running"}, false); err != nil {
		t.Fatal(err)
	}
	if err := PrintPollWait(&out, 1500*time.Millisecond, 2500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := PrintPollResult(&out, 3, &api.Operation{Status: "success"}, true); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	assertContains(t, got, "Polling\n")
	assertContains(t, got, "  Poll 1     queued\n")
	assertContains(t, got, "  Waiting    1s before next poll (elapsed 1s)\n")
	assertContains(t, got, "  Poll 2     running\n")
	assertContains(t, got, "  Waiting    1.5s before next poll (elapsed 2s)\n")
	assertContains(t, got, "  Poll 3     success\n\n")
	assertNoANSI(t, got)
}

func TestColorizedTextUsesANSIWhenNOColorIsUnset(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	var out bytes.Buffer
	err := PrintOperation(&out, &api.Operation{
		MetricSyncID:  "id",
		OperationType: "sync",
		Status:        "success",
		SyncTag:       "tag",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ansiPattern.MatchString(out.String()) {
		t.Fatalf("expected ANSI color output, got %q", out.String())
	}
}

func assertContains(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got %q", want, got)
	}
}

func assertNoANSI(t *testing.T, got string) {
	t.Helper()
	if ansiPattern.MatchString(got) {
		t.Fatalf("expected no ANSI escapes, got %q", got)
	}
}
