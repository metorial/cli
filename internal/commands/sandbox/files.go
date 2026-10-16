package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/spf13/cobra"
)

type filesFlags struct {
	ttlSeconds int
}

func newFilesCommand(ctx commandutil.Context) *cobra.Command {
	flags := &filesFlags{}

	command := &cobra.Command{
		Use:   "files",
		Short: "Browse, download, and upload files on a sandbox machine",
		Long: strings.TrimSpace(`
Browse, download, and upload files on a sandbox machine.

Remote paths are absolute paths on the machine. Recursive transfers (-r) behave like
cp -r: when the destination is an existing directory, the source is copied into it.`),
		Example: strings.TrimSpace(`
  metorial-admin sandbox machines files ls sbm_123 /workspace
  metorial-admin sandbox machines files cat sbm_123 /etc/os-release
  metorial-admin sandbox machines files download sbm_123 /workspace/report.pdf
  metorial-admin sandbox machines files download sbm_123 /workspace ./backup -r
  metorial-admin sandbox machines files upload sbm_123 ./app /workspace -r
  echo hello | metorial-admin sandbox machines files upload sbm_123 - /tmp/hello.txt`),
	}

	command.PersistentFlags().IntVar(&flags.ttlSeconds, "ttl-seconds", 0, "Lifetime of the access token in seconds (default 3600, max 86400)")

	command.AddCommand(
		newLsCommand(ctx, flags),
		newStatCommand(ctx, flags),
		newCatCommand(ctx, flags),
		newDownloadCommand(ctx, flags),
		newUploadCommand(ctx, flags),
		newRmCommand(ctx, flags),
		newMkdirCommand(ctx, flags),
		newMvCommand(ctx, flags),
	)

	return command
}

func withFiles(
	command *cobra.Command,
	ctx commandutil.Context,
	flags *filesFlags,
	machineID string,
	run func(context.Context, *sandboxaccess.FilesClient) error,
) error {
	source, err := newSource(ctx, machineID, sandboxaccess.ScopeFiles, flags.ttlSeconds)
	if err != nil {
		return err
	}

	runCtx, stop := interruptContext(command.Context())
	defer stop()

	client := sandboxaccess.NewFilesClient(source)
	defer client.Close()

	return run(runCtx, client)
}

func newLsCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var recursive bool
	var maxDepth, limit int

	command := &cobra.Command{
		Use:     "ls <machine-id> [path]",
		Aliases: []string{"list"},
		Short:   "List a directory on a sandbox machine",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			target := "/"
			if len(args) > 1 {
				var err error
				if target, err = remotePath(args[1]); err != nil {
					return err
				}
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				entries := []sandboxaccess.FileEntry{}
				result, err := client.List(runCtx, sandboxaccess.ListOptions{
					Path:      target,
					Recursive: recursive,
					MaxDepth:  maxDepth,
					Limit:     limit,
				}, func(batch []sandboxaccess.FileEntry) error {
					entries = append(entries, batch...)
					return nil
				})
				if sandboxaccess.IsFileErrorCode(err, "not_a_directory") {
					entry, statErr := client.Stat(runCtx, target, false)
					if statErr != nil {
						return statErr
					}
					entries = []sandboxaccess.FileEntry{entry}
					result, err = sandboxaccess.ListResult{Count: 1}, nil
				}
				if err != nil {
					return err
				}

				sort.Slice(entries, func(i, j int) bool {
					return entryName(entries[i]) < entryName(entries[j])
				})

				if command.Flags().Changed("format") {
					if err := commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, entries); err != nil {
						return err
					}
				} else if err := writeEntryTable(command.OutOrStdout(), entries); err != nil {
					return err
				}

				if result.Truncated {
					fmt.Fprintf(command.ErrOrStderr(), "metorial: listing stopped after %d entries, raise --limit to see more\n", result.Count)
				}
				return nil
			})
		},
	}

	command.Flags().BoolVarP(&recursive, "recursive", "r", false, "List subdirectories recursively")
	command.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum depth for recursive listings")
	command.Flags().IntVar(&limit, "limit", 0, "Maximum number of entries (default 10000, max 100000)")

	return command
}

func newStatCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var noFollow bool

	command := &cobra.Command{
		Use:   "stat <machine-id> <path>",
		Short: "Show metadata for a file on a sandbox machine",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			target, err := remotePath(args[1])
			if err != nil {
				return err
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				entry, err := client.Stat(runCtx, target, !noFollow)
				if err != nil {
					return err
				}
				return commandutil.WriteValue(command.OutOrStdout(), ctx.App.StdoutFeatures(), ctx.Options.Format, entry)
			})
		},
	}

	command.Flags().BoolVar(&noFollow, "no-follow", false, "Describe a symlink itself instead of its target")

	return command
}

func newCatCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "cat <machine-id> <path>",
		Short: "Print a file from a sandbox machine",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			target, err := remotePath(args[1])
			if err != nil {
				return err
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				_, err := client.Read(runCtx, target, 0, nil, command.OutOrStdout())
				return err
			})
		},
	}
}

func newRmCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var recursive, force bool

	command := &cobra.Command{
		Use:   "rm <machine-id> <path>...",
		Short: "Delete files on a sandbox machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			targets, err := remotePaths(args[1:])
			if err != nil {
				return err
			}
			for _, target := range targets {
				if target == "/" {
					return errors.New("metorial: refusing to delete /")
				}
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				for _, target := range targets {
					if _, err := client.Delete(runCtx, target, recursive, force); err != nil {
						if sandboxaccess.IsFileErrorCode(err, "not_empty") || sandboxaccess.IsFileErrorCode(err, "is_a_directory") {
							return fmt.Errorf("%w; pass -r to delete directories", err)
						}
						return err
					}
				}
				return nil
			})
		},
	}

	command.Flags().BoolVarP(&recursive, "recursive", "r", false, "Delete directories and their contents")
	command.Flags().BoolVarP(&force, "force", "f", false, "Ignore paths that do not exist")

	return command
}

func newMkdirCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var noParents bool
	var mode string

	command := &cobra.Command{
		Use:   "mkdir <machine-id> <path>...",
		Short: "Create directories on a sandbox machine",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			targets, err := remotePaths(args[1:])
			if err != nil {
				return err
			}
			if err := validateMode(mode); err != nil {
				return err
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				for _, target := range targets {
					if _, err := client.Mkdir(runCtx, target, !noParents, mode); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}

	command.Flags().BoolVar(&noParents, "no-parents", false, "Fail instead of creating missing parent directories")
	command.Flags().StringVar(&mode, "mode", "", "Octal permissions for new directories, e.g. 0755")

	return command
}

func newMvCommand(ctx commandutil.Context, flags *filesFlags) *cobra.Command {
	var overwrite bool

	command := &cobra.Command{
		Use:   "mv <machine-id> <from> <to>",
		Short: "Move or rename a file on a sandbox machine",
		Args:  cobra.ExactArgs(3),
		RunE: func(command *cobra.Command, args []string) error {
			paths, err := remotePaths(args[1:])
			if err != nil {
				return err
			}

			return withFiles(command, ctx, flags, args[0], func(runCtx context.Context, client *sandboxaccess.FilesClient) error {
				return client.Move(runCtx, paths[0], paths[1], overwrite)
			})
		},
	}

	command.Flags().BoolVar(&overwrite, "overwrite", false, "Replace the destination if it exists")

	return command
}

func remotePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("metorial: remote path %q must be absolute (start with /)", value)
	}
	return path.Clean(value), nil
}

func remotePaths(values []string) ([]string, error) {
	paths := make([]string, 0, len(values))
	for _, value := range values {
		cleaned, err := remotePath(value)
		if err != nil {
			return nil, err
		}
		paths = append(paths, cleaned)
	}
	return paths, nil
}

func validateMode(mode string) error {
	if mode == "" {
		return nil
	}
	value, err := strconv.ParseUint(strings.TrimPrefix(mode, "0o"), 8, 32)
	if err != nil || value > 0o7777 {
		return fmt.Errorf("metorial: invalid --mode %q, expected octal permissions such as 0644", mode)
	}
	return nil
}

func entryName(entry sandboxaccess.FileEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	return entry.Path
}

func writeEntryTable(writer io.Writer, entries []sandboxaccess.FileEntry) error {
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', tabwriter.AlignRight)
	for _, entry := range entries {
		name := entryName(entry)
		switch entry.Type {
		case "directory":
			name += "/"
		case "symlink":
			if entry.Target != "" {
				name += " -> " + entry.Target
			}
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t %s\n",
			modeString(entry.Type, entry.Mode),
			humanBytes(entry.Size),
			time.UnixMilli(entry.Mtime).Format("2006-01-02 15:04"),
			name,
		)
	}
	return table.Flush()
}

func modeString(kind string, mode uint32) string {
	var builder strings.Builder
	switch kind {
	case "directory":
		builder.WriteByte('d')
	case "symlink":
		builder.WriteByte('l')
	case "file":
		builder.WriteByte('-')
	default:
		builder.WriteByte('?')
	}
	for bit := 8; bit >= 0; bit-- {
		if mode&(1<<uint(bit)) != 0 {
			builder.WriteByte("rwx"[(8-bit)%3])
		} else {
			builder.WriteByte('-')
		}
	}
	return builder.String()
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return strconv.FormatInt(size, 10) + "B"
	}
	value, suffix := float64(size), ""
	for _, next := range []string{"K", "M", "G", "T", "P"} {
		value /= unit
		suffix = next
		if value < unit {
			break
		}
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + suffix
}
