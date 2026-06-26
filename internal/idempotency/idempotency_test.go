package idempotency

import (
	"testing"

	"github.com/DataDog/datadog-experiment-metric-sync-cli/internal/model"
	"github.com/DataDog/datadog-experiment-metric-sync-cli/internal/payload"
)

func TestDeriveStableForSameInput(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_RUN_ID", "123")
	input := Input{
		Operation: "plan",
		Payload: model.SyncConfig{
			SchemaVersion: 1,
			SyncTag:       "checkout",
		},
		Options: payload.SubmitOptions{UpgradeMode: "none"},
	}

	first, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("got %q and %q", first, second)
	}
}

func TestDeriveChangesWithCIIdentity(t *testing.T) {
	clearCIEnv(t)
	input := Input{
		Operation: "execute",
		Payload:   model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout"},
		Options:   payload.SubmitOptions{UpgradeMode: "none"},
	}

	t.Setenv("GITHUB_RUN_ID", "123")
	first, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_RUN_ID", "456")
	second, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("expected key to change with CI identity; got %q", first)
	}
}

func TestDeriveChangesWithoutCIIdentity(t *testing.T) {
	clearCIEnv(t)
	input := Input{
		Operation: "plan",
		Payload:   model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout"},
		Options:   payload.SubmitOptions{UpgradeMode: "none"},
	}

	first, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("expected local keys to be unique per invocation; got %q", first)
	}
}

func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT",
		"CI_PIPELINE_ID", "CI_JOB_ID",
		"BUILDKITE_BUILD_ID",
		"CIRCLE_WORKFLOW_ID",
		"BUILD_BUILDID",
		"BUILD_ID",
	} {
		t.Setenv(key, "")
	}
}
