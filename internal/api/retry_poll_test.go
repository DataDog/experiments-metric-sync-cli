// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
	"github.com/DataDog/experiments-metric-sync-cli/internal/config"
	"github.com/DataDog/experiments-metric-sync-cli/internal/poll"
)

func TestPollingResumesAfterRateLimitAndFetchesResult(t *testing.T) {
	statusReads, resultReads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if strings.HasSuffix(r.URL.Path, "/result") {
			resultReads++
			if resultReads == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"operation-id","attributes":{"status":"success","created":{"metrics":[{"sync_id":"metric","id":"metric-id"}]}}}}`)
			return
		}
		statusReads++
		if statusReads == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		status := "running"
		if statusReads > 2 {
			status = "success"
		}
		_, _ = io.WriteString(w, `{"data":{"id":"operation-id","attributes":{"status":"`+status+`"}}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := api.NewClient(config.Config{BaseURL: server.URL})
	terminal, err := poll.UntilTerminal(ctx, client, "operation-id", time.Millisecond, nil)
	if err != nil {
		t.Fatalf("polling failed instead of resuming after HTTP 429: %v", err)
	}
	result, err := client.GetResult(ctx, terminal.MetricSyncID)
	if err != nil {
		t.Fatal(err)
	}
	if statusReads != 3 || resultReads != 2 || result.Status != "success" || len(result.Created.Metrics) != 1 || result.Created.Metrics[0].SyncID != "metric" {
		t.Fatalf("status reads=%d result reads=%d result=%#v", statusReads, resultReads, result)
	}
}
