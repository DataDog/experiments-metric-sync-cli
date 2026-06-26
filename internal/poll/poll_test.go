// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package poll

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
)

func TestUntilTerminalEmitsPollAndWaitEvents(t *testing.T) {
	client := &fakeStatusClient{
		operations: []*api.Operation{
			{MetricSyncID: "id", OperationType: "sync", Status: "queued", SyncTag: "tag"},
			{MetricSyncID: "id", OperationType: "sync", Status: "running", SyncTag: "tag"},
			{MetricSyncID: "id", OperationType: "sync", Status: "success", SyncTag: "tag"},
		},
	}
	var events []Event

	operation, err := UntilTerminal(context.Background(), client, "id", time.Millisecond, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "success" {
		t.Fatalf("got terminal status %q, want success", operation.Status)
	}

	expected := []struct {
		kind     EventKind
		attempt  int
		status   string
		terminal bool
	}{
		{kind: EventPollResult, attempt: 1, status: "queued"},
		{kind: EventWait, attempt: 1},
		{kind: EventPollResult, attempt: 2, status: "running"},
		{kind: EventWait, attempt: 2},
		{kind: EventPollResult, attempt: 3, status: "success", terminal: true},
	}
	if len(events) != len(expected) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(expected), events)
	}
	for i, want := range expected {
		got := events[i]
		if got.Kind != want.kind || got.Attempt != want.attempt || got.Terminal != want.terminal {
			t.Fatalf("event %d got %+v, want %+v", i, got, want)
		}
		if want.status != "" && got.Operation.Status != want.status {
			t.Fatalf("event %d got status %q, want %q", i, got.Operation.Status, want.status)
		}
		if got.Elapsed < 0 {
			t.Fatalf("event %d got negative elapsed %s", i, got.Elapsed)
		}
		if got.Kind == EventWait && got.Interval != time.Millisecond {
			t.Fatalf("event %d got interval %s, want %s", i, got.Interval, time.Millisecond)
		}
	}
}

func TestUntilTerminalAllowsNilObserver(t *testing.T) {
	client := &fakeStatusClient{
		operations: []*api.Operation{
			{MetricSyncID: "id", OperationType: "plan", Status: "planned", SyncTag: "tag"},
		},
	}

	operation, err := UntilTerminal(context.Background(), client, "id", time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "planned" {
		t.Fatalf("got status %q, want planned", operation.Status)
	}
}

type fakeStatusClient struct {
	operations []*api.Operation
	index      int
}

func (c *fakeStatusClient) GetStatus(ctx context.Context, metricSyncID string) (*api.Operation, error) {
	if c.index >= len(c.operations) {
		return c.operations[len(c.operations)-1], nil
	}
	operation := c.operations[c.index]
	c.index++
	return operation, nil
}
