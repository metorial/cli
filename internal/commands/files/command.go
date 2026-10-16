package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/spf13/cobra"
)

// Attach adds the hand-written upload command under the generated `files`
// command, so it must run after the generated commands exist.
func Attach(root *cobra.Command, ctx commandutil.Context) error {
	files := resourcecmd.CommandByName(root, "files")
	if files == nil {
		return errors.New("metorial: the generated files command is missing")
	}

	command := newUploadCommand(ctx)
	commandutil.ConfigureCommand(command)
	commandutil.SetCommandCategory(command, commandutil.CommandCategoryResource)
	files.AddCommand(command)
	return nil
}

func newUploadCommand(ctx commandutil.Context) *cobra.Command {
	options := uploadOptions{}
	var name string

	command := &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload a local file or directory",
		Long: strings.TrimSpace(`
Upload a local file to Metorial and print the created file.

Directories are zipped before uploading. The archive contains the directory's
contents rather than the directory itself, and is named after the directory with
a .zip extension unless --name is given. Symlinks and special files are skipped.

Examples:
  metorial-admin files upload ./report.pdf
  metorial-admin files upload ./logo.png --purpose organization_image
  metorial-admin files upload ./build-context --purpose sandbox_image_build_context
  metorial-admin files upload ./notes.md --store-id <store-id> --store-path docs/notes.md
`),
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if (options.StoreID == "") != (options.StorePath == "") {
				return errors.New("metorial: --store-id and --store-path must be given together")
			}
			if options.StoreReplace && options.StoreID == "" {
				return errors.New("metorial: --store-replace requires --store-id and --store-path")
			}

			format, err := output.ParseFormat(ctx.Options.Format)
			if err != nil {
				return err
			}
			runtime, err := ctx.ResolveRuntime()
			if err != nil {
				return err
			}
			if err := runtime.RequireAPIKey(); err != nil {
				return err
			}
			options.InstanceID = runtime.InstanceID

			source, cleanup, err := prepareSource(args[0], name, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer cleanup()

			response, uploadErr := newUploader(runtime).upload(command.Context(), source, options)
			if response != nil {
				writer := command.OutOrStdout()
				if uploadErr != nil {
					writer = command.ErrOrStderr()
				}
				if err := output.WriteResponse(writer, response, output.RenderOptions{
					Format: format,
					Colors: ctx.App.StdoutFeatures(),
				}); err != nil {
					return err
				}
			}
			return uploadErr
		},
	}

	command.Flags().StringVar(&options.Purpose, "purpose", "generic", "What the file is for, such as generic, organization_image, or sandbox_image_build_context")
	command.Flags().StringVar(&options.Title, "title", "", "Human-readable title for the file")
	command.Flags().StringVar(&name, "name", "", "File name to store (default: the local file name, or <directory>.zip)")
	command.Flags().StringVar(&options.StoreID, "store-id", "", "Also place the file in this file store")
	command.Flags().StringVar(&options.StorePath, "store-path", "", "Path of the file inside the file store")
	command.Flags().BoolVar(&options.StoreReplace, "store-replace", false, "Replace an existing file at --store-path")

	return command
}

// prepareSource resolves the local path to a single file to upload, zipping
// directories into a temporary archive that cleanup removes.
func prepareSource(local, name string, stderr io.Writer) (uploadSource, func(), error) {
	noop := func() {}

	absolute, err := filepath.Abs(local)
	if err != nil {
		return uploadSource{}, noop, fmt.Errorf("metorial: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return uploadSource{}, noop, fmt.Errorf("metorial: %w", err)
	}

	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return uploadSource{}, noop, fmt.Errorf("metorial: %s is not a regular file", local)
		}
		if name == "" {
			name = filepath.Base(absolute)
		}
		return uploadSource{
			Path:        absolute,
			Name:        name,
			ContentType: contentTypeFor(name),
			Size:        info.Size(),
		}, noop, nil
	}

	archive, err := os.CreateTemp("", "metorial-upload-*.zip")
	if err != nil {
		return uploadSource{}, noop, fmt.Errorf("metorial: %w", err)
	}
	cleanup := func() { _ = os.Remove(archive.Name()) }

	result, err := zipDirectory(absolute, archive, archive.Name())
	if closeErr := archive.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("metorial: %w", closeErr)
	}
	if err != nil {
		cleanup()
		return uploadSource{}, noop, err
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(stderr, "metorial: skipping %s: not a regular file\n", skipped)
	}
	if result.Files == 0 {
		cleanup()
		return uploadSource{}, noop, fmt.Errorf("metorial: %s has no files to upload", local)
	}

	archiveInfo, err := os.Stat(archive.Name())
	if err != nil {
		cleanup()
		return uploadSource{}, noop, fmt.Errorf("metorial: %w", err)
	}

	if name == "" {
		name = filepath.Base(absolute) + ".zip"
	}
	noun := "files"
	if result.Files == 1 {
		noun = "file"
	}
	fmt.Fprintf(stderr, "metorial: zipped %d %s from %s (%s)\n", result.Files, noun, local, commandutil.HumanBytes(archiveInfo.Size()))

	return uploadSource{
		Path:        archive.Name(),
		Name:        name,
		ContentType: "application/zip",
		Size:        archiveInfo.Size(),
	}, cleanup, nil
}
