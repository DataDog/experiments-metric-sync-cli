package poll

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/metric-sync-cli/internal/api"
)

type StatusClient interface {
	GetStatus(ctx context.Context, metricSyncID string) (*api.Operation, error)
}

type EventKind string

const (
	EventPollResult EventKind = "poll_result"
	EventWait       EventKind = "wait"
)

type Event struct {
	Kind      EventKind
	Attempt   int
	Operation *api.Operation
	Interval  time.Duration
	Elapsed   time.Duration
	Terminal  bool
}

type Observer func(Event)

func UntilTerminal(ctx context.Context, client StatusClient, metricSyncID string, interval time.Duration, observer Observer) (*api.Operation, error) {
	if interval <= 0 {
		interval = 2 * time.Second
	}

	start := time.Now()
	attempt := 1

	for {
		operation, err := client.GetStatus(ctx, metricSyncID)
		if err != nil {
			return nil, err
		}
		terminal := api.IsTerminalStatus(operation.Status)
		notify(observer, Event{
			Kind:      EventPollResult,
			Attempt:   attempt,
			Operation: operation,
			Elapsed:   time.Since(start),
			Terminal:  terminal,
		})
		if terminal {
			return operation, nil
		}

		notify(observer, Event{
			Kind:     EventWait,
			Attempt:  attempt,
			Interval: interval,
			Elapsed:  time.Since(start),
		})
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.New("timed out waiting for metric sync operation")
		case <-timer.C:
		}
		attempt++
	}
}

func notify(observer Observer, event Event) {
	if observer != nil {
		observer(event)
	}
}
