//go:build js && wasm

package config

func browserSessionAuthAvailable() bool {
	return true
}
