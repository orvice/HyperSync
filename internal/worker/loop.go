// Package worker provides the shared loop scaffolding for background workers
// so they all stop cleanly when the process shuts down.
package worker

import (
	"context"
	"time"
)

// RunLoop runs fn immediately and then once per interval until ctx is
// cancelled. It never starts a new iteration after cancellation; an iteration
// already in flight is left to finish (fn bounds its own work).
func RunLoop(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}
		fn(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
