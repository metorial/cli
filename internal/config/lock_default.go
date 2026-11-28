//go:build !js

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

const (
	configLockTimeout  = 30 * time.Second
	refreshLockTimeout = 90 * time.Second
	lockRetryDelay     = 25 * time.Millisecond
)

func lockPath(path string, timeout time.Duration) (func(), error) {
	lock := flock.New(path, flock.SetPermissions(0o600))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, lockRetryDelay)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("timed out waiting for %s", path)
	}

	return func() { _ = lock.Unlock() }, nil
}
