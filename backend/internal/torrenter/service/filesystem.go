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
		fs.Logger.Error("Failed to create symbolic link", "error", err)
		return err
	}
	return nil
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
