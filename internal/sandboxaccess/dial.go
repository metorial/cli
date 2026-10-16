package sandboxaccess

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
)

const (
	protocol            = "metorial-sandbox-v1"
	tokenProtocolPrefix = "metorial-sandbox-token."
	readLimit           = 4 << 20

	statusTokenRejected websocket.StatusCode = 4401
)

var ErrTokenRejected = errors.New("metorial: sandbox access token was rejected")

func Subprotocols(token string) []string {
	return []string{protocol, tokenProtocolPrefix + token}
}

func dialGrant(ctx context.Context, grant Grant) (*websocket.Conn, error) {
	target, err := grant.URL()
	if err != nil {
		return nil, err
	}

	conn, response, err := websocket.Dial(ctx, target, &websocket.DialOptions{
		Subprotocols: Subprotocols(grant.Token),
	})
	if err != nil {
		if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
			return nil, fmt.Errorf("%w (%s)", ErrTokenRejected, response.Status)
		}
		return nil, fmt.Errorf("metorial: failed to connect to sandbox %s: %w", grant.Scope, err)
	}

	conn.SetReadLimit(readLimit)
	return conn, nil
}

// Dial connects with the source's current grant. A rejected token is usually an
// expired cached one, so the source is invalidated and the dial retried once.
func Dial(ctx context.Context, source *Source) (*websocket.Conn, error) {
	for attempt := 0; ; attempt++ {
		grant, err := source.Get(ctx)
		if err != nil {
			return nil, err
		}

		conn, err := dialGrant(ctx, grant)
		if err == nil {
			return conn, nil
		}
		if !errors.Is(err, ErrTokenRejected) || attempt > 0 {
			return nil, err
		}
		source.Invalidate()
	}
}
