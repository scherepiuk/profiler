package profiling

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"
)

type EventType int

const (
	EventTypeStart = EventType(iota)
	EventTypeEnd
)

type Event struct {
	Timestamp time.Time
	Type      EventType
	Function  string
}

type Sample struct {
	Timestamp time.Time
	Callstack []string
}

func Run(ctx context.Context, interval, duration time.Duration) ([]Event, error) {
	samples, err := collect(ctx, interval, duration)
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("failed to collect samples: %w", err)
	}

	return transform(samples), nil
}

func collect(ctx context.Context, interval, duration time.Duration) ([]Sample, error) {
	capacity := min(duration/interval, 1024)
	samples := make([]Sample, 0, capacity)

	done := time.After(duration)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case ts := <-ticker.C:
			cs := callstack()
			samples = append(samples, Sample{ts, cs})

		case <-done:
			return samples, nil

		case <-ctx.Done():
			return samples, ctx.Err()
		}
	}
}

func callstack() []string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs) // skip [runtime.Callers], [callstack] and [collect] calls.
	spawn := pcs[:n]

	functions := make([]string, 0)

	frames := runtime.CallersFrames(spawn)
	for {
		frame, ok := frames.Next()
		if !ok {
			break
		}

		functions = append(functions, frame.Func.Name())
	}

	return functions
}

func transform(samples []Sample) []Event {
	events := make([]Event, 0)

	var prev Sample
	for _, curr := range samples {
		removed, added := difference(prev.Callstack, curr.Callstack)

		for _, function := range removed {
			event := Event{
				Timestamp: curr.Timestamp,
				Type:      EventTypeEnd,
				Function:  function,
			}
			events = append(events, event)
		}

		for _, function := range added {
			event := Event{
				Timestamp: curr.Timestamp,
				Type:      EventTypeStart,
				Function:  function,
			}
			events = append(events, event)
		}

		prev = curr
	}

	return events
}

func difference[T comparable](s1, s2 []T) (removed []T, added []T) {
	for _, e := range s1 {
		if !slices.Contains(s2, e) {
			removed = append(removed, e)
		}
	}

	for _, e := range s2 {
		if !slices.Contains(s1, e) {
			added = append(added, e)
		}
	}

	return
}
