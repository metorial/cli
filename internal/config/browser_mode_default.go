//go:build !js

package config

func browserSessionAuthAvailable() bool {
	return false
}
