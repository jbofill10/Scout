package service

import (
	"log"
	"os"
)

type FsSvc struct {
	Logger *log.Logger
}

func NewFsSvc(logger *log.Logger) FileSystem {
	return &FsSvc{Logger: logger}
}

func (fs *FsSvc) HardLink(sourcePath, destPath string) error {
	fs.Logger.Printf("Creating hard link from %s to %s", sourcePath, destPath)
	err := os.Link(sourcePath, destPath)
	if err != nil {
		fs.Logger.Printf("Failed to create hard link: %v", err)
	}
	return err
}

func (fs *FsSvc) MkDir(path string) error {
	fs.Logger.Printf("Creating directory at %s", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fs.Logger.Printf("Failed to create directory: %v", err)
	}
	return err
}
