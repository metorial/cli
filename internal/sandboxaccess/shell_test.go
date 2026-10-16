package sandboxaccess_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/metorial/cli/internal/sandboxaccess/sandboxtest"
)

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func noBackoff(int) time.Duration { return 0 }

func readText(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	for {
		kind, data, err := conn.Read(context.Background())
		if err != nil {
			t.Errorf("read: %v", err)
			return nil
		}
		if kind == websocket.MessageText {
			message := map[string]any{}
			_ = json.Unmarshal(data, &message)
			return message
		}
	}
}

func TestShellExecRelaysStreamsStdinAndExitCode(t *testing.T) {
	t.Parallel()

	server := sandboxtest.New(t)
	stdin := make(chan string, 1)
	server.Shell = func(conn *websocket.Conn, first map[string]any) {
		if first["type"] != "open" || first["tty"] != false || first["cwd"] != "/work" {
			t.Errorf("open frame = %v", first)
		}
		if command, _ := json.Marshal(first["command"]); string(command) != `["sh","-c","cat"]` {
			t.Errorf("command = %s", command)
		}

		sandboxtest.WriteJSON(conn, map[string]any{"type": "session_init", "sessionId": "shs_1", "pid": 7, "tty": false, "resumed": false, "offset": 0})
		_ = conn.Write(context.Background(), websocket.MessageBinary, append([]byte{0x01}, "out"...))
		_ = conn.Write(context.Background(), websocket.MessageBinary, append([]byte{0x02}, "err"...))

		_, data, _ := conn.Read(context.Background())
		stdin <- string(data)
		if eof := readText(t, conn); eof["type"] != "stdin_eof" {
			t.Errorf("expected stdin_eof, got %v", eof)
		}

		sandboxtest.WriteJSON(conn, map[string]any{"type": "exit", "code": 3, "signal": nil})
		_ = conn.Close(websocket.StatusNormalClosure, "exited")
	}

	stdout, stderr := &syncBuffer{}, &syncBuffer{}
	shell, err := sandboxaccess.OpenShell(context.Background(), server.Source(sandboxaccess.ScopeShell), sandboxaccess.ShellOptions{
		Command: []string{"sh", "-c", "cat"},
		Cwd:     "/work",
		Stdout:  stdout,
		Stderr:  stderr,
	})
	if err != nil {
		t.Fatalf("OpenShell() error = %v", err)
	}
	if shell.SessionID() != "shs_1" {
		t.Fatalf("SessionID() = %q", shell.SessionID())
	}

	if _, err := shell.Write([]byte("input")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := shell.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin() error = %v", err)
	}

	exit, err := shell.Wait()
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if exit.ExitCode() != 3 {
		t.Fatalf("ExitCode() = %d, want 3", exit.ExitCode())
	}
	if got := <-stdin; got != "input" {
		t.Fatalf("stdin = %q", got)
	}
	if stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestShellResumesAtTheReceivedOffsetAfterADrop(t *testing.T) {
	t.Parallel()

	server := sandboxtest.New(t)
	attach := make(chan map[string]any, 1)
	server.Shell = func(conn *websocket.Conn, first map[string]any) {
		if first["type"] == "open" {
			sandboxtest.WriteJSON(conn, map[string]any{"type": "session_init", "sessionId": "shs_9", "tty": true, "offset": 10})
			_ = conn.Write(context.Background(), websocket.MessageBinary, []byte("hello"))
			readText(t, conn)
			return
		}

		attach <- first
		sandboxtest.WriteJSON(conn, map[string]any{"type": "session_init", "sessionId": "shs_9", "tty": true, "resumed": true, "offset": 15})
		_ = conn.Write(context.Background(), websocket.MessageBinary, []byte(" world"))
		sandboxtest.WriteJSON(conn, map[string]any{"type": "exit", "code": 0, "signal": nil})
		_ = conn.Close(websocket.StatusNormalClosure, "exited")
	}

	stdout := &syncBuffer{}
	notices := &syncBuffer{}
	shell, err := sandboxaccess.OpenShell(context.Background(), server.Source(sandboxaccess.ScopeShell), sandboxaccess.ShellOptions{
		TTY:     true,
		Stdout:  stdout,
		Notice:  func(message string) { notices.Write([]byte(message + "\n")) },
		Backoff: noBackoff,
	})
	if err != nil {
		t.Fatalf("OpenShell() error = %v", err)
	}

	waitFor(t, func() bool { return stdout.String() == "hello" })
	if err := shell.Signal("int"); err != nil {
		t.Fatalf("Signal() error = %v", err)
	}

	exit, err := shell.Wait()
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if exit.ExitCode() != 0 {
		t.Fatalf("ExitCode() = %d", exit.ExitCode())
	}

	request := <-attach
	if request["type"] != "attach" || request["sessionId"] != "shs_9" || request["offset"] != float64(15) {
		t.Fatalf("attach frame = %v", request)
	}
	if stdout.String() != "hello world" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(notices.String(), "reconnected") {
		t.Fatalf("notices = %q", notices.String())
	}
}

func TestShellReportsSessionsThatEndedWhileDisconnected(t *testing.T) {
	t.Parallel()

	server := sandboxtest.New(t)
	server.Shell = func(conn *websocket.Conn, first map[string]any) {
		if first["type"] == "open" {
			sandboxtest.WriteJSON(conn, map[string]any{"type": "session_init", "sessionId": "shs_2", "tty": true, "offset": 0})
			return
		}
		_ = conn.Close(4404, "session_not_found")
	}

	shell, err := sandboxaccess.OpenShell(context.Background(), server.Source(sandboxaccess.ScopeShell), sandboxaccess.ShellOptions{TTY: true, Backoff: noBackoff})
	if err != nil {
		t.Fatalf("OpenShell() error = %v", err)
	}

	_, err = shell.Wait()
	var shellErr *sandboxaccess.ShellError
	if !errors.As(err, &shellErr) || shellErr.Status != 4404 {
		t.Fatalf("Wait() error = %v, want close status 4404", err)
	}
	if server.ShellConnections() != 2 {
		t.Fatalf("connections = %d, want 2 (no retry after 4404)", server.ShellConnections())
	}
}

func TestDialMintsANewTokenWhenTheCachedOneIsRejected(t *testing.T) {
	t.Parallel()

	server := sandboxtest.New(t)
	source := server.Source(sandboxaccess.ScopeFiles)

	conn, err := sandboxaccess.Dial(context.Background(), source)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	conn.CloseNow()

	server.ExpireTokens()
	conn, err = sandboxaccess.Dial(context.Background(), source)
	if err != nil {
		t.Fatalf("Dial() after expiry error = %v", err)
	}
	conn.CloseNow()

	if server.Minted() != 2 {
		t.Fatalf("minted = %d, want 2", server.Minted())
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
