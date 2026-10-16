//go:build js

package sandbox

import (
	"context"
	"os"
)

const shellSupported = false

func watchResize(context.Context, *os.File, func(cols, rows int)) {}

func forwardSignals(context.Context, func(name string)) {}

func interruptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}
