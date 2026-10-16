package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/spf13/cobra"
)

const (
	transferWorkers   = 8
	transferRetries   = 2
	maxRecursiveItems = 100000
)

func newDownloadCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var recursive bool

	command := &cobra.Command{
		Use:   "download <machine-id> <remote-path> [local-path]",
		Short: "Download files from a sandbox machine",
		Long: strings.TrimSpace(`
Download a file or, with -r, a directory from a sandbox machine.

The local path defaults to the remote file name in the current directory. Pass - as
the local path to write a single file to stdout.`),
		Args: cobra.RangeArgs(2, 3),
		RunE: func(command *cobra.Command, args []string) error {
			remote, err := remotePath(args[1])
			if err != nil {
				return err
			}
			local := ""
			if len(args) > 2 {
				local = args[2]
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				if local == "-" {
					if recursive {
						return errors.New("metorial: cannot download recursively to stdout")
					}
					_, err := client.Read(runCtx, remote, 0, nil, command.OutOrStdout())
					return err
				}

				entry, err := client.Stat(runCtx, remote, true)
				if err != nil {
					return err
				}

				transfer := newTransfer(command, ctx)
				if entry.Type == "directory" {
					if !recursive {
						return fmt.Errorf("metorial: %s is a directory, pass -r to download it", remote)
					}
					target, err := directoryTarget(local, remote)
					if err != nil {
						return err
					}
					return transfer.downloadTree(runCtx, client, remote, target)
				}

				target := local
				if target == "" {
					target = path.Base(remote)
				} else if isLocalDirectory(target) {
					target = filepath.Join(target, path.Base(remote))
				}

				transfer.progress.reset(1, entry.Size)
				err = transfer.downloadFile(runCtx, client, remote, target, entry)
				transfer.progress.fileDone()
				return transfer.finish("downloaded", target, err)
			})
		},
	}

	command.Flags().BoolVarP(&recursive, "recursive", "r", false, "Download a directory and its contents")

	return command
}

func newUploadCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var recursive, noOverwrite bool
	var mode string

	command := &cobra.Command{
		Use:   "upload <machine-id> <local-path> <remote-path>",
		Short: "Upload files to a sandbox machine",
		Long: strings.TrimSpace(`
Upload a file or, with -r, a directory to a sandbox machine.

When the remote path is an existing directory or ends in /, the file is placed inside
it. Missing parent directories are created. Pass - as the local path to upload stdin.`),
		Args: cobra.ExactArgs(3),
		RunE: func(command *cobra.Command, args []string) error {
			local := args[1]
			remote, err := remotePath(args[2])
			if err != nil {
				return err
			}
			intoDirectory := strings.HasSuffix(args[2], "/")
			if err := validateMode(mode); err != nil {
				return err
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				if local == "-" {
					if recursive {
						return errors.New("metorial: cannot upload recursively from stdin")
					}
					if intoDirectory {
						return errors.New("metorial: uploading stdin needs a remote file path, not a directory")
					}
					_, err := client.Write(runCtx, sandboxaccess.WriteOptions{Path: remote, Mode: mode, NoOverwrite: noOverwrite}, ctx.App.Stdin)
					return err
				}

				info, err := os.Stat(local)
				if err != nil {
					return fmt.Errorf("metorial: %w", err)
				}
				absolute, err := filepath.Abs(local)
				if err != nil {
					return fmt.Errorf("metorial: %w", err)
				}

				remoteEntry, statErr := client.Stat(runCtx, remote, true)
				if statErr != nil && !sandboxaccess.IsFileErrorCode(statErr, "not_found") {
					return statErr
				}
				remoteIsDirectory := statErr == nil && remoteEntry.Type == "directory"

				transfer := newTransfer(command, ctx)
				if info.IsDir() {
					if !recursive {
						return fmt.Errorf("metorial: %s is a directory, pass -r to upload it", local)
					}
					target := remote
					if remoteIsDirectory {
						target = path.Join(remote, filepath.Base(absolute))
					}
					return transfer.uploadTree(runCtx, client, absolute, target, mode, noOverwrite)
				}

				target := remote
				if remoteIsDirectory || intoDirectory {
					target = path.Join(remote, filepath.Base(absolute))
				}

				transfer.progress.reset(1, info.Size())
				err = transfer.uploadFile(runCtx, client, absolute, target, info, mode, noOverwrite)
				transfer.progress.fileDone()
				return transfer.finish("uploaded", target, err)
			})
		},
	}

	command.Flags().BoolVarP(&recursive, "recursive", "r", false, "Upload a directory and its contents")
	command.Flags().StringVar(&mode, "mode", "", "Octal permissions for uploaded files (default: the local file's permissions)")
	command.Flags().BoolVar(&noOverwrite, "no-overwrite", false, "Fail instead of replacing existing remote files")

	return command
}

type transfer struct {
	stderr   io.Writer
	progress *progress
}

func newTransfer(command *cobra.Command, ctx commandutil.Context) *transfer {
	stderr := command.ErrOrStderr()
	return &transfer{
		stderr:   stderr,
		progress: &progress{writer: stderr, enabled: ctx.App.StderrFeatures().IsTTY},
	}
}

func (t *transfer) notice(format string, args ...any) {
	t.progress.clear()
	fmt.Fprintf(t.stderr, "metorial: "+format+"\n", args...)
}

func (t *transfer) finish(verb, target string, err error) error {
	t.progress.clear()
	if err != nil {
		return err
	}
	files, bytes := t.progress.totals()
	noun := "files"
	if files == 1 {
		noun = "file"
	}
	fmt.Fprintf(t.stderr, "metorial: %s %d %s (%s) to %s\n", verb, files, noun, commandutil.HumanBytes(bytes), target)
	return nil
}

func (t *transfer) downloadTree(ctx context.Context, client *sandboxaccess.FilesClient, remote, target string) error {
	entries := []sandboxaccess.FileEntry{}
	result, err := client.List(ctx, sandboxaccess.ListOptions{Path: remote, Recursive: true, Limit: maxRecursiveItems}, func(batch []sandboxaccess.FileEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if err != nil {
		return err
	}
	if result.Truncated {
		return fmt.Errorf("metorial: %s has more than %d entries, download smaller directories instead", remote, maxRecursiveItems)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("metorial: %w", err)
	}

	files := []sandboxaccess.FileEntry{}
	total := int64(0)
	for _, entry := range entries {
		relative := filepath.FromSlash(entry.Path)
		if !filepath.IsLocal(relative) {
			t.notice("skipping %s: unsafe path", entry.Path)
			continue
		}
		local := filepath.Join(target, relative)

		switch entry.Type {
		case "directory":
			if err := os.MkdirAll(local, 0o755); err != nil {
				return fmt.Errorf("metorial: %w", err)
			}
		case "file":
			files = append(files, entry)
			total += entry.Size
		case "symlink":
			if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
				return fmt.Errorf("metorial: %w", err)
			}
			if err := os.Symlink(entry.Target, local); err != nil {
				t.notice("skipping symlink %s: %v", entry.Path, err)
			}
		default:
			t.notice("skipping %s: not a regular file", entry.Path)
		}
	}

	t.progress.reset(len(files), total)
	err = runParallel(ctx, len(files), func(ctx context.Context, index int) error {
		entry := files[index]
		err := t.downloadFile(ctx, client, path.Join(remote, entry.Path), filepath.Join(target, filepath.FromSlash(entry.Path)), entry)
		t.progress.fileDone()
		return err
	})
	return t.finish("downloaded", target, err)
}

func (t *transfer) downloadFile(ctx context.Context, client *sandboxaccess.FilesClient, remote, target string, entry sandboxaccess.FileEntry) error {
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("metorial: %w", err)
	}

	for attempt := 0; ; attempt++ {
		temp, err := os.CreateTemp(directory, "."+filepath.Base(target)+".metorial-*")
		if err != nil {
			return fmt.Errorf("metorial: %w", err)
		}

		counter := &countingWriter{writer: temp, progress: t.progress}
		_, err = client.Read(ctx, remote, 0, nil, counter)
		if closeErr := temp.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("metorial: %w", closeErr)
		}
		if err == nil {
			err = finalizeDownload(temp.Name(), target, entry)
		}
		if err == nil {
			return nil
		}

		_ = os.Remove(temp.Name())
		t.progress.add(-counter.written)
		if attempt >= transferRetries || !sandboxaccess.IsRetryable(err) || ctx.Err() != nil {
			return err
		}
	}
}

func finalizeDownload(temp, target string, entry sandboxaccess.FileEntry) error {
	if entry.Mode != 0 {
		_ = os.Chmod(temp, fs.FileMode(entry.Mode&0o777))
	}
	if entry.Mtime > 0 {
		modified := time.UnixMilli(entry.Mtime)
		_ = os.Chtimes(temp, modified, modified)
	}
	if err := os.Rename(temp, target); err != nil {
		return fmt.Errorf("metorial: %w", err)
	}
	return nil
}

func (t *transfer) uploadTree(ctx context.Context, client *sandboxaccess.FilesClient, local, remote, mode string, noOverwrite bool) error {
	type upload struct {
		local  string
		remote string
		info   fs.FileInfo
	}

	directories := []string{remote}
	uploads := []upload{}
	total := int64(0)

	err := filepath.WalkDir(local, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == local {
			return nil
		}
		relative, err := filepath.Rel(local, current)
		if err != nil {
			return err
		}
		target := path.Join(remote, filepath.ToSlash(relative))

		switch {
		case entry.IsDir():
			directories = append(directories, target)
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			uploads = append(uploads, upload{local: current, remote: target, info: info})
			total += info.Size()
		default:
			t.notice("skipping %s: not a regular file", current)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("metorial: %w", err)
	}

	if err := runParallel(ctx, len(directories), func(ctx context.Context, index int) error {
		_, err := client.Mkdir(ctx, directories[index], true, "")
		if sandboxaccess.IsFileErrorCode(err, "already_exists") {
			return nil
		}
		return err
	}); err != nil {
		return err
	}

	t.progress.reset(len(uploads), total)
	err = runParallel(ctx, len(uploads), func(ctx context.Context, index int) error {
		item := uploads[index]
		err := t.uploadFile(ctx, client, item.local, item.remote, item.info, mode, noOverwrite)
		t.progress.fileDone()
		return err
	})
	return t.finish("uploaded", remote, err)
}

func (t *transfer) uploadFile(ctx context.Context, client *sandboxaccess.FilesClient, local, remote string, info fs.FileInfo, mode string, noOverwrite bool) error {
	if mode == "" && runtime.GOOS != "windows" {
		mode = fmt.Sprintf("%04o", info.Mode().Perm())
	}

	for attempt := 0; ; attempt++ {
		file, err := os.Open(local)
		if err != nil {
			return fmt.Errorf("metorial: %w", err)
		}

		size := info.Size()
		sent := int64(0)
		_, err = client.Write(ctx, sandboxaccess.WriteOptions{
			Path:        remote,
			Size:        &size,
			Mode:        mode,
			NoOverwrite: noOverwrite,
			Progress: func(written int64) {
				t.progress.add(written - sent)
				sent = written
			},
		}, file)
		_ = file.Close()
		if err == nil {
			return nil
		}

		t.progress.add(-sent)
		if attempt >= transferRetries || !sandboxaccess.IsRetryable(err) || ctx.Err() != nil {
			return err
		}
	}
}

func directoryTarget(local, remote string) (string, error) {
	if local == "" {
		if remote == "/" {
			return "", errors.New("metorial: pass a local directory when downloading /")
		}
		return path.Base(remote), nil
	}
	if isLocalDirectory(local) && remote != "/" {
		return filepath.Join(local, path.Base(remote)), nil
	}
	return local, nil
}

func isLocalDirectory(value string) bool {
	info, err := os.Stat(value)
	return err == nil && info.IsDir()
}

func runParallel(ctx context.Context, count int, run func(context.Context, int) error) error {
	if count == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	var once sync.Once
	var firstErr error
	var wait sync.WaitGroup

	for worker := 0; worker < min(transferWorkers, count); worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				if err := run(ctx, index); err != nil {
					once.Do(func() {
						firstErr = err
						cancel()
					})
				}
			}
		}()
	}

dispatch:
	for index := 0; index < count; index++ {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	wait.Wait()

	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

type countingWriter struct {
	writer   io.Writer
	progress *progress
	written  int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	w.progress.add(int64(n))
	return n, err
}

type progress struct {
	writer  io.Writer
	enabled bool

	mu        sync.Mutex
	files     int
	filesDone int
	total     int64
	done      int64
	rendered  time.Time
	visible   bool
}

func (p *progress) reset(files int, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.files, p.filesDone, p.total, p.done = files, 0, total, 0
}

func (p *progress) add(n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done += n
	p.render(false)
}

func (p *progress) fileDone() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.filesDone++
	p.render(p.filesDone == p.files)
}

func (p *progress) totals() (int, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.filesDone, p.done
}

func (p *progress) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.visible {
		fmt.Fprint(p.writer, "\r\x1b[K")
		p.visible = false
	}
}

func (p *progress) render(force bool) {
	if !p.enabled || (!force && time.Since(p.rendered) < 100*time.Millisecond) {
		return
	}
	p.rendered = time.Now()

	line := commandutil.HumanBytes(p.done)
	if p.total > 0 {
		line += fmt.Sprintf(" / %s (%d%%)", commandutil.HumanBytes(p.total), p.done*100/p.total)
	}
	if p.files > 1 {
		line = fmt.Sprintf("%d/%d files  %s", p.filesDone, p.files, line)
	}
	fmt.Fprint(p.writer, "\r\x1b[K"+line)
	p.visible = true
}
