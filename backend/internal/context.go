package internal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Context provides the context.Context instance that's closed when SIGINT or SIGTERM will be sent
// It can be used for grace shutdown of the application
func Context() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-sigs:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	return ctx, cancel
}
