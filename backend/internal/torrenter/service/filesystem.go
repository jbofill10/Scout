package service

import (
	"fmt"
	iofs "io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

type FsSvc struct {
	Logger *slog.Logger
}

func NewFsSvc(logger *slog.Logger) FileSystem {
	return &FsSvc{Logger: logger}
}

func (fs *FsSvc) HardLink(sourcePath, destPath string) error {
	fs.Logger.Info("Creating symbolic link", "source", sourcePath, "dest", destPath)

	// Check if source exists first to maintain hard link behavior
	if _, err := os.Stat(sourcePath); err != nil {
		fs.Logger.Error("Source file does not exist", "source", sourcePath, "error", err)
		return fmt.Errorf("source file does not exist: %w", err)
	}

	err := os.Symlink(sourcePath, destPath)
	if err != nil {
		// Linking the same file twice is a success, not a failure. A resumed
		// monitor can replay a completion event that was already processed —
		// if the process died between the event and its bookkeeping — and that
		// must not mark a download that actually landed as failed.
		if os.IsExist(err) && fs.linksTo(destPath, sourcePath) {
			fs.Logger.Info("Link already present, treating as success", "source", sourcePath, "dest", destPath)
			return nil
		}
		fs.Logger.Error("Failed to create symbolic link", "error", err)
		return err
	}
	return nil
}

// linksTo reports whether destPath already points at sourcePath, so an existing
// link can be told apart from a genuine collision with different content.
func (fs *FsSvc) linksTo(destPath, sourcePath string) bool {
	target, err := os.Readlink(destPath)
	if err != nil {
		return false
	}
	return target == sourcePath
}

func (fs *FsSvc) MkDir(path string) error {
	fs.Logger.Info("Creating directory", "path", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fs.Logger.Error("Failed to create directory", "error", err)
	}
	return err
}

// WalkFiles returns every file underneath root, with paths relative to root.
// It recurses because qBittorrent nests multi-file torrents inside their own
// root folder, so the media file is rarely at the top level of the save path.
func (fs *FsSvc) WalkFiles(root string) ([]models.FileEntry, error) {
	var files []models.FileEntry

	err := filepath.WalkDir(root, func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		var size int64
		if info, infoErr := entry.Info(); infoErr == nil {
			size = info.Size()
		}

		files = append(files, models.FileEntry{Path: relPath, Size: size})
		return nil
	})
	if err != nil {
		fs.Logger.Error("Failed to walk directory", "path", root, "error", err)
		return nil, err
	}

	return files, nil
}

// WriteFile writes data to a temp file beside path and renames it into place,
// so a reader never sees a partial file. It refuses to replace an existing
// path: a subtitle the user put there by hand must win over a fetched one.
func (fs *FsSvc) WriteFile(path string, data []byte) error {
	if fs.Exists(path) {
		return fmt.Errorf("%s already exists", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Exists reports whether path exists, following nothing: a dangling link
// still counts, since the name is taken either way.
func (fs *FsSvc) Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Remove deletes one file; a path that is already gone is not an error.
func (fs *FsSvc) Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove %s: %w", path, err)
	}
	return nil
}
