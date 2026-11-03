// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/config"
	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"github.com/DataDog/experiments-metric-sync-cli/internal/payload"
)

func TestRetryAfterDelay(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
		valid bool
	}{
		{"60", time.Minute, true},
		{" 0 ", 0, true},
		{now.Add(time.Minute).Format(http.TimeFormat), time.Minute, true},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0, true},
		{"", 0, false}, {"invalid", 0, false}, {"-1", 0, false}, {"1.5", 0, false},
		{"999999999999999999999999999", maxRateLimitWait + time.Second, true},
		{"9223372036854775807", maxRateLimitWait + time.Second, true},
		{now.Add(24 * time.Hour).Format(http.TimeFormat), maxRateLimitWait + time.Second, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, valid := retryAfterDelay(tc.value, now)
			if got != tc.want || valid != tc.valid {
				t.Fatalf("delay=%s valid=%v, want %s %v", got, valid, tc.want, tc.valid)
			}
		})
	}
}

func TestRateLimitDelayGuidanceAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, retryAfter, reset string
		attempt                 int
		minimum, maximum        time.Duration
	}{
		{"retry-after", "60", "", 1, time.Minute, time.Minute + time.Second},
		{"reset is relative seconds", "", "45", 1, 45 * time.Second, 46 * time.Second},
		{"longer reset", "10", "60", 1, time.Minute, time.Minute + time.Second},
		{"longer retry-after", "60", "10", 1, time.Minute, time.Minute + time.Second},
		{"invalid retry-after valid reset", "invalid", "60", 1, time.Minute, time.Minute + time.Second},
		{"missing", "", "", 1, time.Second, 2 * time.Second},
		{"invalid", "invalid", "-1", 2, 2 * time.Second, 4 * time.Second},
		{"negative", "-1", "", 3, 4 * time.Second, 8 * time.Second},
		{"capped", "", "", 7, 15 * time.Second, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Retry-After", tc.retryAfter)
			headers.Set("X-RateLimit-Reset", tc.reset)
			for i := 0; i < 20; i++ {
				if got := rateLimitDelay(headers, tc.attempt, time.Now()); got < tc.minimum || got >= tc.maximum {
					t.Fatalf("delay %s outside [%s, %s)", got, tc.minimum, tc.maximum)
				}
			}
		})
	}
}

func TestSubmitRetriesIdenticalBodyAndIdempotencyKey(t *testing.T) {
	var bodies, queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, string(body))
		queries = append(queries, r.URL.RawQuery)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/experiments/metric-syncs" || r.Header.Get("Idempotency-Key") != "caller-key" || r.Header.Get("DD-API-KEY") != "api" || r.Header.Get("DD-APPLICATION-KEY") != "app" {
			t.Errorf("unexpected submit: %s %s headers=%v", r.Method, r.URL.Path, r.Header)
		}
		if len(bodies) < 3 {
			w.Header().Set("X-RateLimit-Reset", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"data":{"id":"operation-id","attributes":{"status":"queued"}}}`)
	}))
	defer server.Close()
	client, waits := newRetryTestClient(server.URL)
	got, err := client.Submit(context.Background(), "plan", model.SyncConfig{SchemaVersion: 1, SyncTag: "checkout", Metrics: []model.Metric{{SyncID: "caller-metric"}}}, payload.SubmitOptions{IsCertified: true}, "caller-key")
	if err != nil || got == nil || got.MetricSyncID != "operation-id" {
		t.Fatalf("operation=%#v error=%v", got, err)
	}
	if len(bodies) != 3 || len(*waits) != 2 {
		t.Fatalf("bodies=%v waits=%v", bodies, *waits)
	}
	for i := range bodies {
		if bodies[i] != bodies[0] || queries[i] != queries[0] || !strings.Contains(bodies[i], `"sync_id":"caller-metric"`) {
			t.Fatalf("request changed: bodies=%v queries=%v", bodies, queries)
		}
	}
	for _, wait := range *waits {
		if wait < time.Minute || wait >= time.Minute+time.Second {
			t.Errorf("wait=%s", wait)
		}
	}
}

func newRetryTestClient(site string) (*Client, *[]time.Duration) {
	var waits []time.Duration
	client := NewClient(config.Config{BaseURL: site, APIKey: "api", AppKey: "app"})
	client.retrySleep = func(ctx context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return ctx.Err()
	}
	return client, &waits
}

func TestRateLimitRetryBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, guidance string
		attempts       int
	}{
		{"attempt limit", "", 8},
		{"wait limit", "100", 2},
		{"too large", "181", 1},
		{"overflow", "999999999999999999999999", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", tc.guidance)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"errors":[{"title":"rate limit"}]}`)
			}))
			defer server.Close()
			client, waits := newRetryTestClient(server.URL)
			_, err := client.GetStatus(context.Background(), "operation-id")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || !strings.Contains(err.Error(), "retry budget exhausted") || !strings.Contains(err.Error(), "rate limit") {
				t.Fatalf("error=%v", err)
			}
			if calls != tc.attempts || len(*waits) != calls-1 {
				t.Fatalf("calls=%d waits=%v", calls, *waits)
			}
			var total time.Duration
			for _, wait := range *waits {
				total += wait
			}
			if total > maxRateLimitWait {
				t.Fatalf("total wait=%s", total)
			}
		})
	}
}

func TestWarehouseLookupRetries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/unstable/ffe/warehouse-connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"connection-id","attributes":{"name":"Warehouse"}}]}`)
	}))
	defer server.Close()
	client, waits := newRetryTestClient(server.URL)
	got, err := client.ListWarehouseConnections(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != "connection-id" || calls != 2 || len(*waits) != 1 {
		t.Fatalf("connections=%v calls=%d waits=%v error=%v", got, calls, *waits, err)
	}
}

func TestRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewClient(config.Config{BaseURL: server.URL})
	client.retrySleep = func(ctx context.Context, delay time.Duration) error {
		close(waiting)
		return waitForRetry(ctx, delay)
	}
	done := make(chan error, 1)
	go func() { _, err := client.GetStatus(ctx, "operation-id"); done <- err }()
	select {
	case <-waiting:
		cancel()
	case <-time.After(3 * time.Second):
		t.Fatal("did not begin retry wait")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("error=%v calls=%d", err, calls)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt wait")
	}
}

func TestNonRateLimitResponsesAreNotRetried(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500, 503, 202} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"data":{"attributes":{"message":"pending"}}}`)
			}))
			defer server.Close()
			client, waits := newRetryTestClient(server.URL)
			_, err := client.GetResult(context.Background(), "operation-id")
			if calls != 1 || len(*waits) != 0 {
				t.Fatalf("calls=%d waits=%v", calls, *waits)
			}
			if status == 202 {
				var pending *ResultNotReadyError
				if !errors.As(err, &pending) || pending.Message != "pending" || pending.RetryAfter != "60" {
					t.Fatalf("pending error=%v", err)
				}
			} else {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
					t.Fatalf("API error=%v", err)
				}
			}
		})
	}
}

type retryTestTransport func(*http.Request) (*http.Response, error)

func (f retryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedRetryBody struct {
	io.Reader
	closed bool
}

func (b *trackedRetryBody) Close() error { b.closed = true; return nil }

func TestRetryClosesResponseBeforeWaiting(t *testing.T) {
	client, _ := newRetryTestClient("http://example.invalid")
	body := &trackedRetryBody{Reader: strings.NewReader("rate limited")}
	calls := 0
	transportErr := errors.New("transport failed")
	client.httpClient.Transport = retryTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 429, Body: body, Header: http.Header{}}, nil
		}
		return nil, transportErr
	})
	client.retrySleep = func(ctx context.Context, delay time.Duration) error {
		if !body.closed {
			t.Error("response body open during wait")
		}
		return nil
	}
	_, err := client.GetStatus(context.Background(), "operation-id")
	if !errors.Is(err, transportErr) || calls != 2 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}
