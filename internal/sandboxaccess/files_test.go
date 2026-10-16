package sandboxaccess_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/metorial/cli/internal/sandboxaccess/sandboxtest"
)

func newFilesClient(t *testing.T) (*sandboxtest.Server, *sandboxaccess.FilesClient) {
	t.Helper()
	server := sandboxtest.New(t)
	client := sandboxaccess.NewFilesClient(server.Source(sandboxaccess.ScopeFiles))
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func TestFilesWriteAndReadRoundTripInChunks(t *testing.T) {
	t.Parallel()

	server, client := newFilesClient(t)
	ctx := context.Background()

	payload := bytes.Repeat([]byte("0123456789abcdef"), 10_000)
	size := int64(len(payload))
	progress := int64(0)
	result, err := client.Write(ctx, sandboxaccess.WriteOptions{
		Path:     "/work/data.bin",
		Size:     &size,
		Mode:     "0600",
		Progress: func(written int64) { progress = written },
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	sum := sha256.Sum256(payload)
	if result.Bytes != size || result.SHA256 != hex.EncodeToString(sum[:]) || progress != size {
		t.Fatalf("Write() = %+v, progress = %d", result, progress)
	}
	if node, ok := server.Node("/work/data.bin"); !ok || node.Mode != 0o600 {
		t.Fatalf("stored node = %+v, %v", node, ok)
	}

	var start sandboxaccess.ReadStart
	output := &bytes.Buffer{}
	read, err := client.Read(ctx, "/work/data.bin", 0, func(value sandboxaccess.ReadStart) error {
		start = value
		return nil
	}, output)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read != size || start.Size != size || !bytes.Equal(output.Bytes(), payload) {
		t.Fatalf("Read() = %d bytes, start = %+v, equal = %v", read, start, bytes.Equal(output.Bytes(), payload))
	}
}

func TestFilesListCollectsEveryBatch(t *testing.T) {
	t.Parallel()

	server, client := newFilesClient(t)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		server.PutFile("/dir/"+name, []byte(name), 0o644)
	}
	server.PutFile("/dir/sub/f", []byte("f"), 0o644)

	entries := []sandboxaccess.FileEntry{}
	result, err := client.List(context.Background(), sandboxaccess.ListOptions{Path: "/dir"}, func(batch []sandboxaccess.FileEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Count != 6 || len(entries) != 6 || entries[5].Name != "sub" || entries[5].Type != "directory" {
		t.Fatalf("List() = %+v, entries = %+v", result, entries)
	}
}

func TestFilesErrorsCarryTheServerCode(t *testing.T) {
	t.Parallel()

	_, client := newFilesClient(t)
	_, err := client.Stat(context.Background(), "/missing", true)
	if !sandboxaccess.IsFileErrorCode(err, "not_found") {
		t.Fatalf("Stat() error = %v, want not_found", err)
	}
	if sandboxaccess.IsRetryable(err) {
		t.Fatal("not_found should not be retryable")
	}
}

func TestFilesMultiplexesConcurrentReads(t *testing.T) {
	t.Parallel()

	server, client := newFilesClient(t)
	for index := 0; index < 30; index++ {
		server.PutFile(fmt.Sprintf("/f/%d", index), bytes.Repeat([]byte{byte('a' + index%26)}, 50+index), 0o644)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 30)
	for index := 0; index < 30; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			output := &bytes.Buffer{}
			if _, err := client.Read(context.Background(), fmt.Sprintf("/f/%d", index), 0, nil, output); err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(output.Bytes(), bytes.Repeat([]byte{byte('a' + index%26)}, 50+index)) {
				errs <- fmt.Errorf("file %d content mismatch: %q", index, output.String())
			}
		}(index)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestFilesInterruptedOpsAreRetryableAndRedial(t *testing.T) {
	t.Parallel()

	server, client := newFilesClient(t)
	server.PutFile("/x", []byte("payload"), 0o644)
	server.FailNextRead = true

	_, err := client.Read(context.Background(), "/x", 0, nil, &bytes.Buffer{})
	if !sandboxaccess.IsRetryable(err) || !sandboxaccess.IsFileErrorCode(err, "connection_interrupted") {
		t.Fatalf("Read() error = %v, want retryable connection_interrupted", err)
	}

	output := &bytes.Buffer{}
	if _, err := client.Read(context.Background(), "/x", 0, nil, output); err != nil {
		t.Fatalf("Read() after redial error = %v", err)
	}
	if output.String() != "payload" {
		t.Fatalf("output = %q", output.String())
	}
}
