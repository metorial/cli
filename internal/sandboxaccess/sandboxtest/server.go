// Package sandboxtest fakes the sandbox access API and the shell and files
// WebSocket endpoints for tests.
package sandboxtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/metorial/cli/internal/sandboxaccess"
)

const readChunk = 3

type Node struct {
	Dir   bool
	Data  []byte
	Mode  uint32
	Mtime int64
}

type ShellHandler func(conn *websocket.Conn, first map[string]any)

type Server struct {
	*httptest.Server

	t      testing.TB
	mu     sync.Mutex
	nodes  map[string]*Node
	tokens map[string]bool
	minted int
	shells int

	Shell          ShellHandler
	FailNextRead   bool
	AccessRequests []string
}

func New(t testing.TB) *Server {
	server := &Server{
		t:      t,
		nodes:  map[string]*Node{"/": {Dir: true, Mode: 0o755}},
		tokens: map[string]bool{},
	}
	server.Server = httptest.NewServer(http.HandlerFunc(server.route))
	t.Cleanup(server.Close)
	return server
}

func (s *Server) WebSocketURL(scope sandboxaccess.Scope) string {
	return "ws" + strings.TrimPrefix(s.URL, "http") + "/" + string(scope)
}

func (s *Server) Source(scope sandboxaccess.Scope) *sandboxaccess.Source {
	return sandboxaccess.NewSource(func(ctx context.Context) (sandboxaccess.Grant, error) {
		var grant sandboxaccess.Grant
		err := json.Unmarshal(s.mint(scope), &grant)
		return grant, err
	})
}

func (s *Server) ExpireTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = map[string]bool{}
}

func (s *Server) Minted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.minted
}

func (s *Server) ShellConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shells
}

func (s *Server) PutFile(name string, data []byte, mode uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureParents(name)
	s.nodes[name] = &Node{Data: append([]byte(nil), data...), Mode: mode, Mtime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli()}
}

func (s *Server) PutDir(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureParents(name)
	s.nodes[name] = &Node{Dir: true, Mode: 0o755}
}

func (s *Server) Node(name string) (Node, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.nodes[name]
	if !ok {
		return Node{}, false
	}
	return *node, true
}

func (s *Server) ensureParents(name string) {
	for parent := path.Dir(name); parent != "/" && parent != "."; parent = path.Dir(parent) {
		if _, ok := s.nodes[parent]; !ok {
			s.nodes[parent] = &Node{Dir: true, Mode: 0o755}
		}
	}
}

func (s *Server) mint(scope sandboxaccess.Scope) []byte {
	s.mu.Lock()
	s.minted++
	token := fmt.Sprintf("tok_%d", s.minted)
	s.tokens[token] = true
	s.mu.Unlock()

	url := s.WebSocketURL(scope)
	urls := map[string]any{"type": "single", "shell": nil, "files": nil, "ports": nil}
	urls[string(scope)] = url
	body, _ := json.Marshal(map[string]any{
		"object":     "sandbox.machine.access",
		"token":      token,
		"scope":      scope,
		"urls":       urls,
		"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	return body
}

func (s *Server) route(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/sandbox/machines/"):
		parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
		if len(parts) != 5 || parts[3] != "access" {
			http.NotFound(writer, request)
			return
		}
		s.mu.Lock()
		s.AccessRequests = append(s.AccessRequests, parts[2]+":"+parts[4])
		s.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(s.mint(sandboxaccess.Scope(parts[4])))
	case request.URL.Path == "/shell":
		s.serveShell(writer, request)
	case request.URL.Path == "/files":
		s.serveFiles(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) accept(writer http.ResponseWriter, request *http.Request) *websocket.Conn {
	authorized := false
	for _, value := range request.Header.Values("Sec-WebSocket-Protocol") {
		for _, protocol := range strings.Split(value, ",") {
			token, ok := strings.CutPrefix(strings.TrimSpace(protocol), "metorial-sandbox-token.")
			s.mu.Lock()
			if ok && s.tokens[token] {
				authorized = true
			}
			s.mu.Unlock()
		}
	}
	if !authorized {
		http.Error(writer, `{"code":"token_invalid"}`, http.StatusUnauthorized)
		return nil
	}

	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{Subprotocols: []string{"metorial-sandbox-v1"}})
	if err != nil {
		s.t.Errorf("accept: %v", err)
		return nil
	}
	conn.SetReadLimit(4 << 20)
	return conn
}

func (s *Server) serveShell(writer http.ResponseWriter, request *http.Request) {
	conn := s.accept(writer, request)
	if conn == nil {
		return
	}
	defer conn.CloseNow()

	s.mu.Lock()
	s.shells++
	handler := s.Shell
	s.mu.Unlock()

	_, data, err := conn.Read(request.Context())
	if err != nil {
		return
	}
	first := map[string]any{}
	_ = json.Unmarshal(data, &first)
	if handler != nil {
		handler(conn, first)
	}
}

func WriteJSON(conn *websocket.Conn, value any) {
	payload, _ := json.Marshal(value)
	_ = conn.Write(context.Background(), websocket.MessageText, payload)
}

type pendingWrite struct {
	path   string
	mode   uint32
	data   []byte
	append bool
}

func (s *Server) serveFiles(writer http.ResponseWriter, request *http.Request) {
	conn := s.accept(writer, request)
	if conn == nil {
		return
	}
	defer conn.CloseNow()

	ctx := request.Context()
	writes := map[string]*pendingWrite{}
	reply := func(id string, kind string, fields map[string]any) {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["id"] = id
		fields["type"] = kind
		WriteJSON(conn, fields)
	}
	fail := func(id, code, message string) {
		reply(id, "error", map[string]any{"error": map[string]any{"code": code, "message": message}})
	}

	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		if kind == websocket.MessageBinary {
			id := string(data[1 : 1+int(data[0])])
			if pending := writes[id]; pending != nil {
				pending.data = append(pending.data, data[1+int(data[0]):]...)
			}
			continue
		}

		message := map[string]any{}
		_ = json.Unmarshal(data, &message)
		id, _ := message["id"].(string)
		name, _ := message["path"].(string)

		if control, ok := message["type"].(string); ok {
			pending := writes[id]
			delete(writes, id)
			if pending == nil || control != "end" {
				continue
			}
			sum := sha256.Sum256(pending.data)
			digest := hex.EncodeToString(sum[:])
			if expected, _ := message["sha256"].(string); expected != "" && expected != digest {
				fail(id, "checksum_mismatch", "checksum mismatch")
				continue
			}
			s.mu.Lock()
			s.ensureParents(pending.path)
			node := &Node{Data: pending.data, Mode: pending.mode, Mtime: time.Now().UnixMilli()}
			if existing, ok := s.nodes[pending.path]; ok && pending.append {
				node.Data = append(append([]byte(nil), existing.Data...), pending.data...)
			}
			s.nodes[pending.path] = node
			s.mu.Unlock()
			reply(id, "result", map[string]any{"bytes": len(pending.data), "sha256": digest})
			continue
		}

		switch message["op"] {
		case "stat":
			node, ok := s.Node(name)
			if !ok {
				fail(id, "not_found", "No such file or directory")
				continue
			}
			reply(id, "result", map[string]any{"entry": entry("path", name, node)})
		case "list":
			node, ok := s.Node(name)
			if !ok {
				fail(id, "not_found", "No such file or directory")
				continue
			}
			if !node.Dir {
				fail(id, "not_a_directory", "Not a directory")
				continue
			}
			recursive, _ := message["recursive"].(bool)
			entries := s.children(name, recursive)
			for start := 0; start < len(entries); start += 2 {
				end := min(start+2, len(entries))
				reply(id, "entries", map[string]any{"entries": entries[start:end]})
			}
			reply(id, "result", map[string]any{"count": len(entries), "truncated": false})
		case "read":
			s.mu.Lock()
			failRead := s.FailNextRead
			s.FailNextRead = false
			s.mu.Unlock()
			if failRead {
				return
			}
			node, ok := s.Node(name)
			if !ok {
				fail(id, "not_found", "No such file or directory")
				continue
			}
			if node.Dir {
				fail(id, "not_a_file", "only regular files can be read")
				continue
			}
			reply(id, "start", map[string]any{"size": len(node.Data), "mtime": node.Mtime, "offset": 0})
			for start := 0; start < len(node.Data); start += readChunk {
				end := min(start+readChunk, len(node.Data))
				frame := append([]byte{byte(len(id))}, id...)
				frame = append(frame, node.Data[start:end]...)
				_ = conn.Write(ctx, websocket.MessageBinary, frame)
			}
			reply(id, "result", map[string]any{"bytes": len(node.Data)})
		case "write":
			if overwrite, ok := message["overwrite"].(bool); ok && !overwrite {
				if _, exists := s.Node(name); exists {
					fail(id, "already_exists", "File exists")
					continue
				}
			}
			mode := uint32(0o644)
			if raw, ok := message["mode"].(string); ok {
				var parsed uint32
				_, _ = fmt.Sscanf(raw, "%o", &parsed)
				mode = parsed
			}
			appendMode, _ := message["append"].(bool)
			writes[id] = &pendingWrite{path: name, mode: mode, append: appendMode}
		case "delete":
			s.mu.Lock()
			_, ok := s.nodes[name]
			for key := range s.nodes {
				if key == name || strings.HasPrefix(key, name+"/") {
					delete(s.nodes, key)
				}
			}
			s.mu.Unlock()
			if !ok && message["missingOk"] != true {
				fail(id, "not_found", "No such file or directory")
				continue
			}
			reply(id, "result", map[string]any{"deleted": ok})
		case "mkdir":
			_, exists := s.Node(name)
			if !exists {
				s.PutDir(name)
			}
			reply(id, "result", map[string]any{"created": !exists})
		case "move":
			from, _ := message["from"].(string)
			to, _ := message["to"].(string)
			s.mu.Lock()
			node, ok := s.nodes[from]
			if ok {
				delete(s.nodes, from)
				s.nodes[to] = node
			}
			s.mu.Unlock()
			if !ok {
				fail(id, "not_found", "No such file or directory")
				continue
			}
			reply(id, "result", nil)
		default:
			fail(id, "unknown_op", "unknown op")
		}
	}
}

func (s *Server) children(root string, recursive bool) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := []string{}
	for name := range s.nodes {
		if name == root {
			continue
		}
		prefix := strings.TrimSuffix(root, "/") + "/"
		relative, ok := strings.CutPrefix(name, prefix)
		if !ok || (!recursive && strings.Contains(relative, "/")) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	entries := []map[string]any{}
	for _, name := range names {
		relative := strings.TrimPrefix(name, strings.TrimSuffix(root, "/")+"/")
		if recursive {
			entries = append(entries, entry("path", relative, *s.nodes[name]))
		} else {
			entries = append(entries, entry("name", relative, *s.nodes[name]))
		}
	}
	return entries
}

func entry(key, name string, node Node) map[string]any {
	kind := "file"
	if node.Dir {
		kind = "directory"
	}
	return map[string]any{key: name, "type": kind, "size": len(node.Data), "mode": node.Mode, "mtime": node.Mtime}
}
