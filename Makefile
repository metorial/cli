CLI_NAME := metorial
BIN_DIR := bin
BIN := $(BIN_DIR)/$(CLI_NAME)
PKG := ./cmd/metorial

.PHONY: help build run fmt test tidy check install clean completion-bash completion-zsh completion-fish completion-powershell release snapshot

help:
	@printf "%s\n" \
		"Available targets:" \
		"  make build                 Build the CLI into ./bin/metorial" \
		"  make run                   Run the CLI locally" \
		"  make fmt                   Format all Go code" \
		"  make test                  Run CLI tests" \
		"  make tidy                  Sync go.mod and go.sum" \
		"  make check                 Run fmt, tidy, and tests" \
		"  make install               Install the CLI into GOPATH/bin" \
		"  make clean                 Remove local build output" \
		"  make completion-bash       Print bash completions" \
		"  make completion-zsh        Print zsh completions" \
		"  make completion-fish       Print fish completions" \
		"  make completion-powershell Print PowerShell completions" \
		"  make snapshot              Build a Goreleaser snapshot" \
		"  make release               Run Goreleaser release --clean"

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(PKG)

run:
	go run $(PKG)

fmt:
	gofmt -w $$(find . -name '*.go' -print)

test:
	go test ./...

tidy:
	go mod tidy

check: fmt tidy test

install:
	go install $(PKG)

clean:
	rm -rf $(BIN_DIR)

completion-bash:
	go run $(PKG) completion bash

completion-zsh:
	go run $(PKG) completion zsh

completion-fish:
	go run $(PKG) completion fish

completion-powershell:
	go run $(PKG) completion powershell

snapshot:
	goreleaser release --clean --snapshot

release:
	goreleaser release --clean
