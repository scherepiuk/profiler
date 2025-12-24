package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/scherepiuk/profiler/internal/profiling"
)

func main() {
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	wg.Go(func() { profilingRun(ctx) })

	result := first()
	slog.Info("computation result", "result", result)
}

func profilingRun(ctx context.Context) {
	events, err := profiling.Run(ctx, 10*time.Nanosecond, time.Minute)
	if err != nil {
		log.Fatal(err)
	}

	for _, event := range events {
		slog.Info(
			"profiling event",
			"ts", event.Timestamp,
			"type", event.Type,
			"func", event.Function.String(),
		)
	}
}

func first() int {
	time.Sleep(250 * time.Nanosecond)
	return second()
}

func second() int {
	time.Sleep(500 * time.Nanosecond)
	return third()
}

func third() int {
	time.Sleep(500 * time.Nanosecond)
	return 42
}
