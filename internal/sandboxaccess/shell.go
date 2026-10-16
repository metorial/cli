package sandboxaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	streamStdout = 0x01
	streamStderr = 0x02

	defaultMaxReconnects = 5
	handshakeTimeout     = 20 * time.Second
)

var ErrShellClosed = errors.New("metorial: sandbox shell is closed")

type ShellOptions struct {
	TTY     bool
	Cols    int
	Rows    int
	Command []string
	Cwd     string
	Env     map[string]string

	Stdout io.Writer
	Stderr io.Writer

	// Notice receives human-readable status lines (reconnects, lost output,
	// idle warnings) that should not be mixed into the session's output.
	Notice func(message string)

	MaxReconnects int
	Backoff       func(attempt int) time.Duration
}

type ExitStatus struct {
	Code   *int    `json:"code"`
	Signal *string `json:"signal"`
}

func (e ExitStatus) ExitCode() int {
	if e.Code != nil {
		return *e.Code
	}
	if e.Signal != nil {
		return 128 + signalNumber(*e.Signal)
	}
	return 0
}

type ShellError struct {
	Status websocket.StatusCode
	Reason string
}

func (e *ShellError) Error() string {
	message := shellCloseMessages[e.Status]
	if message == "" {
		message = "connection closed"
	}
	if e.Reason != "" && !strings.Contains(message, e.Reason) {
		message += " (" + e.Reason + ")"
	}
	return "metorial: sandbox shell " + message
}

var shellCloseMessages = map[websocket.StatusCode]string{
	4400: "rejected the request",
	4404: "session was not found",
	4409: "session is attached from another connection",
	4410: "session has ended",
	4500: "failed to start the command",
	4503: "has too many sessions open",
	4000: "was detached",
	1012: "machine is unavailable",
}

type shellMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Offset    int64           `json:"offset"`
	From      int64           `json:"from"`
	To        int64           `json:"to"`
	Code      json.RawMessage `json:"code"`
	Signal    *string         `json:"signal"`
	Message   string          `json:"message"`
	Notice    string          `json:"notice"`
	Reason    string          `json:"reason"`
	Idle      int64           `json:"idleInSeconds"`
}

type shellLink struct {
	conn *websocket.Conn
	gone chan struct{}
}

type Shell struct {
	source *Source
	opts   ShellOptions
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	link      *shellLink
	changed   chan struct{}
	sessionID string
	offset    int64
	exit      *ExitStatus
	closing   bool

	done chan struct{}
	err  error
}

func OpenShell(ctx context.Context, source *Source, opts ShellOptions) (*Shell, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = opts.Stdout
	}
	if opts.Notice == nil {
		opts.Notice = func(string) {}
	}
	if opts.MaxReconnects == 0 {
		opts.MaxReconnects = defaultMaxReconnects
	}
	if opts.Backoff == nil {
		opts.Backoff = defaultBackoff
	}

	open := map[string]any{"type": "open", "v": 1, "tty": opts.TTY}
	if opts.TTY {
		open["cols"] = clampDimension(opts.Cols, 80)
		open["rows"] = clampDimension(opts.Rows, 24)
	}
	if len(opts.Command) > 0 {
		open["command"] = opts.Command
	}
	if opts.Cwd != "" {
		open["cwd"] = opts.Cwd
	}
	if len(opts.Env) > 0 {
		open["env"] = opts.Env
	}

	sessionCtx, cancel := context.WithCancel(context.Background())
	shell := &Shell{
		source:  source,
		opts:    opts,
		ctx:     sessionCtx,
		cancel:  cancel,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}

	link, err := shell.handshake(ctx, open)
	if err != nil {
		cancel()
		return nil, err
	}
	shell.setLink(link)

	go shell.run(link)
	return shell, nil
}

func (s *Shell) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func (s *Shell) Done() <-chan struct{} {
	return s.done
}

func (s *Shell) Wait() (ExitStatus, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exit != nil {
		return *s.exit, nil
	}
	return ExitStatus{}, s.err
}

func (s *Shell) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := s.send(websocket.MessageBinary, append([]byte(nil), p...)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *Shell) Resize(cols, rows int) error {
	return s.sendJSON(map[string]any{"type": "resize", "cols": clampDimension(cols, 80), "rows": clampDimension(rows, 24)})
}

func (s *Shell) Signal(name string) error {
	return s.sendJSON(map[string]any{"type": "signal", "signal": strings.TrimPrefix(strings.ToUpper(name), "SIG")})
}

func (s *Shell) CloseStdin() error {
	return s.sendJSON(map[string]any{"type": "stdin_eof"})
}

// Close ends the connection with a normal closure, which kills the remote
// session; dropped connections are detached and resumed instead.
func (s *Shell) Close() error {
	s.mu.Lock()
	s.closing = true
	link := s.link
	s.mu.Unlock()

	if link != nil {
		_ = link.conn.Close(websocket.StatusNormalClosure, "")
	}
	s.cancel()
	<-s.done
	return nil
}

func (s *Shell) sendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.send(websocket.MessageText, payload)
}

func (s *Shell) send(kind websocket.MessageType, payload []byte) error {
	for {
		s.mu.Lock()
		link, changed := s.link, s.changed
		s.mu.Unlock()

		if link != nil {
			if err := link.conn.Write(s.ctx, kind, payload); err == nil {
				return nil
			}
			select {
			case <-link.gone:
				continue
			case <-s.done:
				return ErrShellClosed
			}
		}

		select {
		case <-changed:
		case <-s.done:
			return ErrShellClosed
		}
	}
}

func (s *Shell) setLink(link *shellLink) {
	s.mu.Lock()
	s.link = link
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

func (s *Shell) dropLink(link *shellLink) {
	s.mu.Lock()
	if s.link == link {
		s.link = nil
	}
	s.mu.Unlock()
	close(link.gone)
}

func (s *Shell) finish(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.cancel()
	close(s.done)
}

func (s *Shell) handshake(ctx context.Context, first map[string]any) (*shellLink, error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, err := Dial(ctx, s.source)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(first)
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		conn.CloseNow()
		return nil, s.closeError(err)
	}

	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			conn.CloseNow()
			return nil, s.closeError(err)
		}
		if kind != websocket.MessageText {
			continue
		}

		var message shellMessage
		if err := json.Unmarshal(data, &message); err != nil {
			continue
		}
		switch message.Type {
		case "session_init":
			s.mu.Lock()
			s.sessionID = message.SessionID
			s.offset = message.Offset
			s.mu.Unlock()
			return &shellLink{conn: conn, gone: make(chan struct{})}, nil
		case "error":
			conn.CloseNow()
			return nil, fmt.Errorf("metorial: sandbox shell rejected the request: %s", message.Message)
		default:
			s.handleText(message)
		}
	}
}

func (s *Shell) run(link *shellLink) {
	for {
		err := s.read(link)
		s.dropLink(link)

		s.mu.Lock()
		exited, closing := s.exit != nil, s.closing
		s.mu.Unlock()
		if exited || closing {
			s.finish(nil)
			return
		}

		if !resumable(err) {
			s.finish(s.closeError(err))
			return
		}

		next, err := s.reconnect(err)
		if err != nil {
			s.finish(err)
			return
		}
		link = next
		s.setLink(link)
	}
}

func (s *Shell) read(link *shellLink) error {
	for {
		kind, data, err := link.conn.Read(s.ctx)
		if err != nil {
			return err
		}

		if kind == websocket.MessageBinary {
			s.handleOutput(data)
			continue
		}

		var message shellMessage
		if err := json.Unmarshal(data, &message); err == nil {
			s.handleText(message)
		}
	}
}

func (s *Shell) handleOutput(data []byte) {
	if s.opts.TTY {
		s.addOffset(len(data))
		_, _ = s.opts.Stdout.Write(data)
		return
	}
	if len(data) == 0 {
		return
	}

	s.addOffset(len(data) - 1)
	switch data[0] {
	case streamStderr:
		_, _ = s.opts.Stderr.Write(data[1:])
	default:
		_, _ = s.opts.Stdout.Write(data[1:])
	}
}

func (s *Shell) addOffset(n int) {
	s.mu.Lock()
	s.offset += int64(n)
	s.mu.Unlock()
}

func (s *Shell) handleText(message shellMessage) {
	switch message.Type {
	case "session_init":
		s.mu.Lock()
		s.sessionID = message.SessionID
		s.offset = message.Offset
		s.mu.Unlock()
	case "output_gap":
		s.mu.Lock()
		s.offset = message.To
		s.mu.Unlock()
		s.opts.Notice(fmt.Sprintf("%d bytes of output were lost while disconnected", message.To-message.From))
	case "exit":
		exit := ExitStatus{Signal: message.Signal}
		var code int
		if len(message.Code) > 0 && json.Unmarshal(message.Code, &code) == nil {
			exit.Code = &code
		}
		s.mu.Lock()
		s.exit = &exit
		s.mu.Unlock()
	case "error":
		s.opts.Notice("sandbox shell error: " + message.Message)
	case "connection_notice":
		switch message.Notice {
		case "idle_warning":
			s.opts.Notice(fmt.Sprintf("connection will close in %ds without activity", message.Idle))
		case "closing":
			s.opts.Notice("connection closing: " + message.Reason)
		}
	}
}

func (s *Shell) reconnect(cause error) (*shellLink, error) {
	s.mu.Lock()
	sessionID := s.sessionID
	s.mu.Unlock()

	if websocket.CloseStatus(cause) == statusTokenRejected {
		s.source.Invalidate()
	}

	lastErr := s.closeError(cause)
	for attempt := 0; attempt < s.opts.MaxReconnects; attempt++ {
		s.opts.Notice(fmt.Sprintf("connection lost, reconnecting (%d/%d)...", attempt+1, s.opts.MaxReconnects))

		select {
		case <-time.After(s.opts.Backoff(attempt)):
		case <-s.ctx.Done():
			return nil, ErrShellClosed
		}

		s.mu.Lock()
		offset := s.offset
		s.mu.Unlock()

		link, err := s.handshake(s.ctx, map[string]any{"type": "attach", "v": 1, "sessionId": sessionID, "offset": offset})
		if err == nil {
			s.opts.Notice("reconnected")
			return link, nil
		}
		lastErr = err

		var shellErr *ShellError
		if errors.As(err, &shellErr) && !resumableStatus(shellErr.Status) {
			return nil, err
		}
		if errors.Is(err, ErrTokenRejected) || (shellErr != nil && shellErr.Status == statusTokenRejected) {
			s.source.Invalidate()
		}
	}
	return nil, lastErr
}

func (s *Shell) closeError(err error) error {
	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		return &ShellError{Status: closeErr.Code, Reason: closeErr.Reason}
	}
	if errors.Is(err, context.Canceled) {
		return ErrShellClosed
	}
	return fmt.Errorf("metorial: sandbox shell connection failed: %w", err)
}

func resumable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	return resumableStatus(websocket.CloseStatus(err))
}

func resumableStatus(status websocket.StatusCode) bool {
	switch status {
	case -1, websocket.StatusGoingAway, websocket.StatusAbnormalClosure, websocket.StatusInternalError,
		websocket.StatusServiceRestart, websocket.StatusTryAgainLater, 4001, statusTokenRejected:
		return true
	}
	return false
}

func defaultBackoff(attempt int) time.Duration {
	delay := 500 * time.Millisecond << attempt
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func clampDimension(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	if value > 1000 {
		return 1000
	}
	return value
}

var signalNumbers = map[string]int{
	"HUP": 1, "INT": 2, "QUIT": 3, "KILL": 9, "USR1": 10, "USR2": 12, "TERM": 15,
}

func signalNumber(name string) int {
	if number, ok := signalNumbers[strings.TrimPrefix(strings.ToUpper(name), "SIG")]; ok {
		return number
	}
	return 0
}
