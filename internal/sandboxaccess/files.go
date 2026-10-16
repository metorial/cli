package sandboxaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

const (
	maxFilesInFlight = 64
	fileChunkSize    = 64 << 10
)

type FileEntry struct {
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	Mode        uint32 `json:"mode"`
	Mtime       int64  `json:"mtime"`
	Target      string `json:"target,omitempty"`
	InvalidUTF8 bool   `json:"invalidUtf8,omitempty"`
}

type FileError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Path      string `json:"path,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (e *FileError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("metorial: %s: %s (%s)", e.Path, e.Message, e.Code)
	}
	return fmt.Sprintf("metorial: %s (%s)", e.Message, e.Code)
}

func IsFileErrorCode(err error, code string) bool {
	var fileErr *FileError
	return errors.As(err, &fileErr) && fileErr.Code == code
}

func IsRetryable(err error) bool {
	var fileErr *FileError
	return errors.As(err, &fileErr) && fileErr.Retryable
}

type ListOptions struct {
	Path      string
	Recursive bool
	MaxDepth  int
	Limit     int
}

type ListResult struct {
	Count     int  `json:"count"`
	Truncated bool `json:"truncated"`
}

type ReadStart struct {
	Size   int64 `json:"size"`
	Mtime  int64 `json:"mtime"`
	Offset int64 `json:"offset"`
}

type WriteOptions struct {
	Path        string
	Size        *int64
	Mode        string
	NoOverwrite bool
	NoParents   bool
	Append      bool
	Sync        bool
	Progress    func(written int64)
}

type WriteResult struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type filesMessage struct {
	ID      *string     `json:"id"`
	Type    string      `json:"type"`
	Error   *FileError  `json:"error"`
	Entries []FileEntry `json:"entries"`
}

type filesConn struct {
	ws   *websocket.Conn
	done chan struct{}
}

type fileOp struct {
	id      string
	conn    *filesConn
	onText  func(kind string, raw []byte)
	onChunk func(payload []byte) error

	once     sync.Once
	done     chan struct{}
	raw      []byte
	err      error
	localErr error
}

func (op *fileOp) complete(raw []byte, err error) {
	op.once.Do(func() {
		op.raw = raw
		op.err = err
		if op.localErr != nil {
			op.err = op.localErr
		}
		close(op.done)
	})
}

type FilesClient struct {
	source *Source
	sem    chan struct{}
	nextID atomic.Uint64

	mu      sync.Mutex
	current *filesConn
	ops     map[string]*fileOp
	closed  bool
}

func NewFilesClient(source *Source) *FilesClient {
	return &FilesClient{
		source: source,
		sem:    make(chan struct{}, maxFilesInFlight),
		ops:    map[string]*fileOp{},
	}
}

func (c *FilesClient) Close() error {
	c.mu.Lock()
	c.closed = true
	current := c.current
	c.current = nil
	c.mu.Unlock()

	if current != nil {
		_ = current.ws.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}

func (c *FilesClient) Stat(ctx context.Context, path string, followSymlinks bool) (FileEntry, error) {
	raw, err := c.run(ctx, map[string]any{"op": "stat", "path": path, "followSymlinks": followSymlinks}, &fileOp{}, nil)
	if err != nil {
		return FileEntry{}, err
	}
	var result struct {
		Entry FileEntry `json:"entry"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return FileEntry{}, fmt.Errorf("metorial: failed to decode stat result: %w", err)
	}
	return result.Entry, nil
}

func (c *FilesClient) List(ctx context.Context, opts ListOptions, onEntries func([]FileEntry) error) (ListResult, error) {
	request := map[string]any{"op": "list", "path": opts.Path, "recursive": opts.Recursive}
	if opts.MaxDepth > 0 {
		request["maxDepth"] = opts.MaxDepth
	}
	if opts.Limit > 0 {
		request["limit"] = opts.Limit
	}

	op := &fileOp{}
	op.onText = func(kind string, raw []byte) {
		if kind != "entries" || onEntries == nil || op.localErr != nil {
			return
		}
		var batch filesMessage
		if err := json.Unmarshal(raw, &batch); err != nil {
			op.localErr = fmt.Errorf("metorial: failed to decode list entries: %w", err)
		} else if err := onEntries(batch.Entries); err != nil {
			op.localErr = err
		}
		if op.localErr != nil {
			c.cancel(op)
		}
	}

	raw, err := c.run(ctx, request, op, nil)
	if err != nil {
		return ListResult{}, err
	}
	var result ListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ListResult{}, fmt.Errorf("metorial: failed to decode list result: %w", err)
	}
	return result, nil
}

func (c *FilesClient) Read(ctx context.Context, path string, offset int64, onStart func(ReadStart) error, writer io.Writer) (int64, error) {
	request := map[string]any{"op": "read", "path": path}
	if offset > 0 {
		request["offset"] = offset
	}

	op := &fileOp{}
	op.onText = func(kind string, raw []byte) {
		if kind != "start" || onStart == nil || op.localErr != nil {
			return
		}
		var start ReadStart
		if err := json.Unmarshal(raw, &start); err != nil {
			op.localErr = fmt.Errorf("metorial: failed to decode read start: %w", err)
		} else if err := onStart(start); err != nil {
			op.localErr = err
		}
		if op.localErr != nil {
			c.cancel(op)
		}
	}
	op.onChunk = func(payload []byte) error {
		_, err := writer.Write(payload)
		return err
	}

	raw, err := c.run(ctx, request, op, nil)
	if err != nil {
		return 0, err
	}
	var result struct {
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, fmt.Errorf("metorial: failed to decode read result: %w", err)
	}
	return result.Bytes, nil
}

func (c *FilesClient) Write(ctx context.Context, opts WriteOptions, reader io.Reader) (WriteResult, error) {
	request := map[string]any{
		"op":        "write",
		"path":      opts.Path,
		"overwrite": !opts.NoOverwrite,
		"parents":   !opts.NoParents,
		"append":    opts.Append,
		"sync":      opts.Sync,
	}
	if opts.Size != nil {
		request["size"] = *opts.Size
	}
	if opts.Mode != "" {
		request["mode"] = opts.Mode
	}

	raw, err := c.run(ctx, request, &fileOp{}, func(op *fileOp) error {
		return c.stream(ctx, op, reader, opts.Progress)
	})
	if err != nil {
		return WriteResult{}, err
	}
	var result WriteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return WriteResult{}, fmt.Errorf("metorial: failed to decode write result: %w", err)
	}
	return result, nil
}

func (c *FilesClient) Delete(ctx context.Context, path string, recursive, missingOK bool) (bool, error) {
	raw, err := c.run(ctx, map[string]any{"op": "delete", "path": path, "recursive": recursive, "missingOk": missingOK}, &fileOp{}, nil)
	if err != nil {
		return false, err
	}
	var result struct {
		Deleted bool `json:"deleted"`
	}
	_ = json.Unmarshal(raw, &result)
	return result.Deleted, nil
}

func (c *FilesClient) Mkdir(ctx context.Context, path string, parents bool, mode string) (bool, error) {
	request := map[string]any{"op": "mkdir", "path": path, "parents": parents}
	if mode != "" {
		request["mode"] = mode
	}
	raw, err := c.run(ctx, request, &fileOp{}, nil)
	if err != nil {
		return false, err
	}
	var result struct {
		Created bool `json:"created"`
	}
	_ = json.Unmarshal(raw, &result)
	return result.Created, nil
}

func (c *FilesClient) Move(ctx context.Context, from, to string, overwrite bool) error {
	_, err := c.run(ctx, map[string]any{"op": "move", "from": from, "to": to, "overwrite": overwrite}, &fileOp{}, nil)
	return err
}

func (c *FilesClient) stream(ctx context.Context, op *fileOp, reader io.Reader, progress func(int64)) error {
	header := append([]byte{byte(len(op.id))}, op.id...)
	buffer := make([]byte, len(header)+fileChunkSize)
	copy(buffer, header)

	hash := sha256.New()
	written := int64(0)
	abort := func(err error) error {
		_ = c.writeJSON(context.Background(), op.conn, map[string]any{"id": op.id, "type": "abort"})
		return err
	}

	for {
		select {
		case <-op.done:
			return nil
		case <-ctx.Done():
			return abort(ctx.Err())
		default:
		}

		n, readErr := io.ReadFull(reader, buffer[len(header):])
		if n > 0 {
			hash.Write(buffer[len(header) : len(header)+n])
			if err := op.conn.ws.Write(ctx, websocket.MessageBinary, buffer[:len(header)+n]); err != nil {
				return interrupted(err)
			}
			written += int64(n)
			if progress != nil {
				progress(written)
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return abort(readErr)
		}
	}

	return c.writeJSON(ctx, op.conn, map[string]any{"id": op.id, "type": "end", "sha256": hex.EncodeToString(hash.Sum(nil))})
}

func (c *FilesClient) run(ctx context.Context, request map[string]any, op *fileOp, stream func(*fileOp) error) ([]byte, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.sem }()

	conn, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}

	op.id = "c" + strconv.FormatUint(c.nextID.Add(1), 36)
	op.conn = conn
	op.done = make(chan struct{})
	request["id"] = op.id

	c.mu.Lock()
	c.ops[op.id] = op
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.ops, op.id)
		c.mu.Unlock()
	}()

	if err := c.writeJSON(ctx, conn, request); err != nil {
		return nil, interrupted(err)
	}

	if stream != nil {
		if err := stream(op); err != nil {
			select {
			case <-op.done:
				if op.err != nil {
					return nil, op.err
				}
			default:
			}
			return nil, err
		}
	}

	select {
	case <-op.done:
		return op.raw, op.err
	case <-ctx.Done():
		c.cancel(op)
		return nil, ctx.Err()
	}
}

func (c *FilesClient) cancel(op *fileOp) {
	go func() {
		_ = c.writeJSON(context.Background(), op.conn, map[string]any{
			"id":     "x" + strconv.FormatUint(c.nextID.Add(1), 36),
			"op":     "cancel",
			"target": op.id,
		})
	}()
}

func (c *FilesClient) writeJSON(ctx context.Context, conn *filesConn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.ws.Write(ctx, websocket.MessageText, payload)
}

func (c *FilesClient) connect(ctx context.Context) (*filesConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, errors.New("metorial: sandbox files client is closed")
	}
	if c.current != nil {
		select {
		case <-c.current.done:
		default:
			return c.current, nil
		}
	}

	ws, err := Dial(ctx, c.source)
	if err != nil {
		return nil, err
	}
	conn := &filesConn{ws: ws, done: make(chan struct{})}
	c.current = conn
	go c.readLoop(conn)
	return conn, nil
}

func (c *FilesClient) readLoop(conn *filesConn) {
	var err error
	for {
		var kind websocket.MessageType
		var data []byte
		kind, data, err = conn.ws.Read(context.Background())
		if err != nil {
			break
		}
		if kind == websocket.MessageBinary {
			c.handleChunk(data)
		} else {
			c.handleText(data)
		}
	}

	if websocket.CloseStatus(err) == statusTokenRejected {
		c.source.Invalidate()
	}
	close(conn.done)

	c.mu.Lock()
	pending := []*fileOp{}
	for _, op := range c.ops {
		if op.conn == conn {
			pending = append(pending, op)
		}
	}
	c.mu.Unlock()

	for _, op := range pending {
		op.complete(nil, interrupted(err))
	}
}

func (c *FilesClient) lookup(id string) *fileOp {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ops[id]
}

func (c *FilesClient) handleChunk(data []byte) {
	if len(data) < 1 || len(data) < 1+int(data[0]) {
		return
	}
	idLength := int(data[0])
	op := c.lookup(string(data[1 : 1+idLength]))
	if op == nil || op.onChunk == nil || op.localErr != nil {
		return
	}
	if err := op.onChunk(data[1+idLength:]); err != nil {
		op.localErr = err
		c.cancel(op)
	}
}

func (c *FilesClient) handleText(data []byte) {
	var message filesMessage
	if err := json.Unmarshal(data, &message); err != nil || message.ID == nil {
		return
	}
	op := c.lookup(*message.ID)
	if op == nil {
		return
	}

	switch message.Type {
	case "result":
		op.complete(data, nil)
	case "error":
		fileErr := message.Error
		if fileErr == nil {
			fileErr = &FileError{Code: "io_error", Message: "unknown error"}
		}
		op.complete(nil, fileErr)
	default:
		if op.onText != nil {
			op.onText(message.Type, data)
		}
	}
}

func interrupted(err error) error {
	message := "the connection to the sandbox was interrupted"
	if err != nil {
		var closeErr websocket.CloseError
		if errors.As(err, &closeErr) && closeErr.Reason != "" {
			message += " (" + closeErr.Reason + ")"
		}
	}
	return &FileError{Code: "connection_interrupted", Message: message, Retryable: true}
}
