package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/metric-sync-cli/internal/config"
)

func TestGetStatusDecodesJSONAPIEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("DD-API-KEY"); got != "api" {
			t.Fatalf("missing api key header")
		}
		if got := r.Header.Get("DD-APPLICATION-KEY"); got != "app" {
			t.Fatalf("missing app key header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":   "operation-id",
				"type": "metric-sync-operations",
				"attributes": map[string]any{
					"sync_tag":       "checkout",
					"operation_type": "plan",
					"status":         "planned",
				},
			},
		})
	}))
	defer server.Close()

	logger := &recordingLogger{}
	client := NewClientWithLogger(config.Config{BaseURL: server.URL, APIKey: "api", AppKey: "app"}, logger)
	got, err := client.GetStatus(context.Background(), "operation-id")
	if err != nil {
		t.Fatal(err)
	}
	if got.MetricSyncID != "operation-id" || got.Status != "planned" || got.SyncTag != "checkout" {
		t.Fatalf("unexpected operation: %#v", got)
	}
	if !logger.contains("http response") {
		t.Fatalf("expected API client to log response details, got %#v", logger.messages)
	}
	if !logger.containsAttr("http response", "status") {
		t.Fatalf("expected API client to log response status, got %#v", logger.entries)
	}
}

func TestGetResultNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"id":"operation-id","attributes":{"message":"not ready"}}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{BaseURL: server.URL, APIKey: "api", AppKey: "app"})
	_, err := client.GetResult(context.Background(), "operation-id")
	if err == nil {
		t.Fatal("expected not ready error")
	}
	notReady, ok := err.(*ResultNotReadyError)
	if !ok {
		t.Fatalf("got %T", err)
	}
	if notReady.RetryAfter != "2" {
		t.Fatalf("got retry-after %q", notReady.RetryAfter)
	}
}

func TestEmptyErrorBodyReturnsUsefulError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(config.Config{BaseURL: server.URL, APIKey: "api", AppKey: "app"})
	_, err := client.GetStatus(context.Background(), "operation-id")
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "Datadog API returned HTTP 401") {
		t.Fatalf("got %q", err.Error())
	}
}

type recordingLogger struct {
	messages []string
	entries  []recordingEntry
}

type recordingEntry struct {
	message string
	attrs   []slog.Attr
}

func (l *recordingLogger) Info(message string, attrs ...slog.Attr) {
	l.messages = append(l.messages, message)
	l.entries = append(l.entries, recordingEntry{message: message, attrs: attrs})
}

func (l *recordingLogger) Error(message string, attrs ...slog.Attr) {
	l.messages = append(l.messages, message)
	l.entries = append(l.entries, recordingEntry{message: message, attrs: attrs})
}

func (l *recordingLogger) contains(message string) bool {
	for _, candidate := range l.messages {
		if candidate == message {
			return true
		}
	}
	return false
}

func (l *recordingLogger) containsAttr(message string, key string) bool {
	for _, entry := range l.entries {
		if entry.message != message {
			continue
		}
		for _, attr := range entry.attrs {
			if attr.Key == key {
				return true
			}
		}
	}
	return false
}
