package files

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type archiveResult struct {
	Files   int
	Skipped []string
}

// zipDirectory writes the contents of directory to destination with paths
// relative to directory, so the directory itself is not a level in the archive.
// exclude is skipped so the archive can live inside the directory being zipped.
func zipDirectory(directory string, destination io.Writer, exclude string) (archiveResult, error) {
	result := archiveResult{}
	archive := zip.NewWriter(destination)

	err := filepath.WalkDir(directory, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == directory || current == exclude {
			return nil
		}

		relative, err := filepath.Rel(directory, current)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)

		if !entry.IsDir() && !entry.Type().IsRegular() {
			result.Skipped = append(result.Skipped, name)
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if entry.IsDir() {
			header.Name = name + "/"
			_, err := archive.CreateHeader(header)
			return err
		}

		header.Name = name
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if err := copyFile(writer, current); err != nil {
			return err
		}
		result.Files++
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("metorial: failed to zip %s: %w", directory, err)
	}

	if err := archive.Close(); err != nil {
		return result, fmt.Errorf("metorial: failed to zip %s: %w", directory, err)
	}
	return result, nil
}

func copyFile(destination io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(destination, file)
	return err
}
