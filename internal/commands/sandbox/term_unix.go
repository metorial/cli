//go:build !windows && !js

package sandbox

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

const shellSupported = true

func watchResize(ctx context.Context, file *os.File, onResize func(cols, rows int)) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case <-signals:
				if cols, rows, err := term.GetSize(int(file.Fd())); err == nil {
					onResize(cols, rows)
				}
			}
		}
	}()
}

func interruptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

func forwardSignals(ctx context.Context, onSignal func(name string)) {
	names := map[os.Signal]string{
		syscall.SIGINT:  "INT",
		syscall.SIGTERM: "TERM",
		syscall.SIGHUP:  "HUP",
		syscall.SIGQUIT: "QUIT",
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case received := <-signals:
				onSignal(names[received])
			}
		}
	}()
}
