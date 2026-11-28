//go:build js

package config

import "time"

const (
	configLockTimeout  = 0
	refreshLockTimeout = 0
)

func lockPath(string, time.Duration) (func(), error) {
	return func() {}, nil
}
