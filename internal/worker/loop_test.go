package worker_test

import (
	"context"
	"testing"
	"time"

	"go.orx.me/apps/hyper-sync/internal/worker"
)

func TestRunLoop_AlreadyCancelled_NeverRunsFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	worker.RunLoop(ctx, time.Hour, func(context.Context) { ran = true })

	if ran {
		t.Fatal("fn must not run when shutdown has already started")
	}
}

func TestRunLoop_RunsAgainOnTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan struct{}, 16)
	go worker.RunLoop(ctx, time.Millisecond, func(context.Context) { calls <- struct{}{} })

	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatalf("fn ran %d times, want at least 2 (immediate + tick)", i)
		}
	}
}

func TestRunLoop_RunsImmediatelyAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		worker.RunLoop(ctx, time.Hour, func(context.Context) { calls <- struct{}{} })
		close(done)
	}()

	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("fn was not run immediately")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoop did not return after ctx was cancelled")
	}
}
