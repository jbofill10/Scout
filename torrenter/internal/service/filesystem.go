package service

import (
	"log/slog"
	"os"
)

type FsSvc struct {
	Logger *slog.Logger
}

func NewFsSvc(logger *slog.Logger) FileSystem {
	return &FsSvc{Logger: logger}
}

func (fs *FsSvc) HardLink(sourcePath, destPath string) error {
	fs.Logger.Info("Creating hard link", "source", sourcePath, "dest", destPath)
	err := os.Link(sourcePath, destPath)
	if err != nil {
		fs.Logger.Error("Failed to create hard link", "error", err)
	}
	return err
}

func (fs *FsSvc) MkDir(path string) error {
	fs.Logger.Info("Creating directory", "path", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fs.Logger.Error("Failed to create directory", "error", err)
	}
	return err
}

func (fs *FsSvc) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		fs.Logger.Error("Failed to read directory", "path", path, "error", err)
		return nil, err
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() {
			filenames = append(filenames, entry.Name())
		}
	}
	return filenames, nil
}
