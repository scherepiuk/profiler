package profiling

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"strings"
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
	Function  Call
}

type Site struct {
	Filename string
	Line     uint64
}

type Call struct {
	Name string
	Site Site
}

func (c *Call) String() string {
	return fmt.Sprintf("%s at %s:%d", c.Name, c.Site.Filename, c.Site.Line)
}

type sample struct {
	timestamp time.Time
	callstack []Call
}

func Run(ctx context.Context, interval, duration time.Duration) ([]Event, error) {
	// Safegruard in case profiling panics (nil dereference, indexing out of bounds etc.)
	defer func() {
		if err := recover(); err != nil {
			slog.Error("profiling paniced, program execution is uninterrupted", "error", err)
		}
	}()

	samples, err := collect(ctx, interval, duration)
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("failed to collect samples: %w", err)
	}

	return transform(samples), nil
}

func collect(ctx context.Context, interval, duration time.Duration) ([]sample, error) {
	capacity := min(duration/interval, 1024)
	samples := make([]sample, 0, capacity)

	done := time.After(duration)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case ts := <-ticker.C:
			cs := callstack()
			samples = append(samples, sample{ts, cs})

		case <-done:
			return samples, nil

		case <-ctx.Done():
			return samples, ctx.Err()
		}
	}
}

func callstack() []Call {
	rawCalls := make([]byte, 1<<20) // 1MB
	n := runtime.Stack(rawCalls, true)

	calls := parseRawCalls(rawCalls[:n])
	slices.Reverse(calls)

	return calls
}

// TOOD: Filter out sleeping goroutines beforehand for execution time calculation?
func parseRawCalls(rawCalls []byte) []Call {
	calls := make([]Call, 0)

	reader := bytes.NewReader(rawCalls)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		text := scanner.Text()

		// Skip everything except function call lines.
		if text == "" || strings.Contains(text, " ") {
			continue
		}

		function := text

		if !scanner.Scan() {
			continue
		}

		text = scanner.Text()
		text = strings.TrimSpace(text)

		index := strings.LastIndexByte(text, ':')
		filename := text[:index] // TODO: Sometimes out of bounds...

		text = text[index+1:]
		text = strings.Split(text, " ")[0]

		line, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			continue // Ignoring the error and skipping the call.
		}

		call := Call{
			Name: function,
			Site: Site{filename, line},
		}
		calls = append(calls, call)
	}

	return calls
}

func transform(samples []sample) []Event {
	events := make([]Event, 0)

	var prev sample
	for _, curr := range samples {
		removed, added := difference(prev.callstack, curr.callstack)
		slices.Reverse(removed)

		for _, function := range removed {
			event := Event{
				Timestamp: curr.timestamp,
				Type:      EventTypeEnd,
				Function:  function,
			}
			events = append(events, event)
		}

		for _, function := range added {
			event := Event{
				Timestamp: curr.timestamp,
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
