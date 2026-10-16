//go:build windows

package sandbox

import (
	"context"
	"os"
	"os/signal"
	"time"

	"golang.org/x/term"
)

const shellSupported = true

func watchResize(ctx context.Context, file *os.File, onResize func(cols, rows int)) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		lastCols, lastRows, _ := term.GetSize(int(file.Fd()))
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(file.Fd()))
				if err == nil && (cols != lastCols || rows != lastRows) {
					lastCols, lastRows = cols, rows
					onResize(cols, rows)
				}
			}
		}
	}()
}

func interruptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt)
}

func forwardSignals(ctx context.Context, onSignal func(name string)) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case <-signals:
				onSignal("INT")
			}
		}
	}()
}
