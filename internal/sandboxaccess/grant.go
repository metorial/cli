package sandboxaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
)

type Scope string

const (
	ScopeShell Scope = "shell"
	ScopeFiles Scope = "files"
)

const refreshBefore = 30 * time.Second

type Grant struct {
	Token     string    `json:"token"`
	Scope     Scope     `json:"scope"`
	URLs      grantURLs `json:"urls"`
	ExpiresAt time.Time `json:"expires_at"`
}

type grantURLs struct {
	Shell *string `json:"shell"`
	Files *string `json:"files"`
}

func (g Grant) URL() (string, error) {
	var value *string
	switch g.Scope {
	case ScopeShell:
		value = g.URLs.Shell
	case ScopeFiles:
		value = g.URLs.Files
	}
	if value == nil || strings.TrimSpace(*value) == "" {
		return "", fmt.Errorf("metorial: sandbox access grant has no %s url", g.Scope)
	}
	return *value, nil
}

type Minter func(ctx context.Context) (Grant, error)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func APIMinter(runtime config.Runtime, machineID string, scope Scope, ttlSeconds int) Minter {
	return func(ctx context.Context) (Grant, error) {
		if err := ctx.Err(); err != nil {
			return Grant{}, err
		}

		data := ""
		if ttlSeconds > 0 {
			data = fmt.Sprintf(`{"ttl_seconds":%d}`, ttlSeconds)
		}

		response, err := fetch.Execute(runtime, fetch.Options{
			Method: "POST",
			Target: "/sandbox/machines/" + url.PathEscape(machineID) + "/access/" + string(scope),
			Data:   data,
		}, strings.NewReader(""))
		if response == nil {
			if err == nil {
				err = errors.New("no response")
			}
			return Grant{}, fmt.Errorf("metorial: failed to create sandbox %s access: %w", scope, err)
		}

		if response.StatusCode < 200 || response.StatusCode >= 300 {
			var decoded apiError
			_ = json.Unmarshal(response.Body, &decoded)
			message := strings.TrimSpace(decoded.Message)
			if message == "" {
				message = response.Status
			}
			return Grant{}, fmt.Errorf("metorial: failed to create sandbox %s access: %s", scope, message)
		}

		var grant Grant
		if err := json.Unmarshal(response.Body, &grant); err != nil {
			return Grant{}, fmt.Errorf("metorial: failed to decode sandbox %s access: %w", scope, err)
		}
		if grant.Scope == "" {
			grant.Scope = scope
		}
		return grant, nil
	}
}

// Source hands out a cached grant, minting a new one when none is cached, the
// cached one is about to expire, or a connection rejected it.
type Source struct {
	mint  Minter
	now   func() time.Time
	mu    sync.Mutex
	grant *Grant
}

func NewSource(mint Minter) *Source {
	return &Source{mint: mint, now: time.Now}
}

func (s *Source) Get(ctx context.Context) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.grant != nil && (s.grant.ExpiresAt.IsZero() || s.now().Add(refreshBefore).Before(s.grant.ExpiresAt)) {
		return *s.grant, nil
	}

	grant, err := s.mint(ctx)
	if err != nil {
		return Grant{}, err
	}
	s.grant = &grant
	return grant, nil
}

func (s *Source) Invalidate() {
	s.mu.Lock()
	s.grant = nil
	s.mu.Unlock()
}
