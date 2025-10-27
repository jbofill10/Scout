package service

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type FsSvcTestSuite struct {
	suite.Suite
	fs     *FsSvc
	logger *slog.Logger
	tmpDir string
}

func TestFsSvcSuite(t *testing.T) {
	suite.Run(t, new(FsSvcTestSuite))
}

func (s *FsSvcTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))
	s.fs = &FsSvc{Logger: s.logger}

	// Create temp directory for testing
	var err error
	s.tmpDir, err = os.MkdirTemp("", "fs-test-*")
	s.Require().NoError(err)
}

func (s *FsSvcTestSuite) TearDownTest() {
	if s.tmpDir != "" {
		os.RemoveAll(s.tmpDir)
	}
}

func (s *FsSvcTestSuite) TestHardLink_Success() {
	// Create source file
	sourcePath := filepath.Join(s.tmpDir, "source.txt")
	err := os.WriteFile(sourcePath, []byte("test content"), 0644)
	s.Require().NoError(err)

	destPath := filepath.Join(s.tmpDir, "dest.txt")

	err = s.fs.HardLink(sourcePath, destPath)

	s.NoError(err)

	// Verify dest file exists
	_, err = os.Stat(destPath)
	s.NoError(err)

	// Verify content is same
	content, err := os.ReadFile(destPath)
	s.NoError(err)
	s.Equal("test content", string(content))
}

func (s *FsSvcTestSuite) TestHardLink_SourceNotExists() {
	sourcePath := filepath.Join(s.tmpDir, "nonexistent.txt")
	destPath := filepath.Join(s.tmpDir, "dest.txt")

	err := s.fs.HardLink(sourcePath, destPath)

	s.Error(err)
}

func (s *FsSvcTestSuite) TestHardLink_DestAlreadyExists() {
	// Create source and dest files
	sourcePath := filepath.Join(s.tmpDir, "source.txt")
	err := os.WriteFile(sourcePath, []byte("source content"), 0644)
	s.Require().NoError(err)

	destPath := filepath.Join(s.tmpDir, "dest.txt")
	err = os.WriteFile(destPath, []byte("dest content"), 0644)
	s.Require().NoError(err)

	// Hard link should fail if dest exists
	err = s.fs.HardLink(sourcePath, destPath)

	s.Error(err)
}

func (s *FsSvcTestSuite) TestHardLink_InvalidPath() {
	sourcePath := filepath.Join(s.tmpDir, "source.txt")
	err := os.WriteFile(sourcePath, []byte("test"), 0644)
	s.Require().NoError(err)

	// Use invalid dest path (directory doesn't exist)
	destPath := "/nonexistent/path/dest.txt"

	err = s.fs.HardLink(sourcePath, destPath)

	s.Error(err)
}

func (s *FsSvcTestSuite) TestMkDir_Success() {
	dirPath := filepath.Join(s.tmpDir, "newdir")

	err := s.fs.MkDir(dirPath)

	s.NoError(err)

	// Verify directory exists
	info, err := os.Stat(dirPath)
	s.NoError(err)
	s.True(info.IsDir())
}

func (s *FsSvcTestSuite) TestMkDir_NestedDirectories() {
	dirPath := filepath.Join(s.tmpDir, "level1", "level2", "level3")

	err := s.fs.MkDir(dirPath)

	s.NoError(err)

	// Verify nested directory exists
	info, err := os.Stat(dirPath)
	s.NoError(err)
	s.True(info.IsDir())
}

func (s *FsSvcTestSuite) TestMkDir_AlreadyExists() {
	dirPath := filepath.Join(s.tmpDir, "existingdir")

	// Create directory first
	err := os.MkdirAll(dirPath, 0755)
	s.Require().NoError(err)

	// MkdirAll should not error if directory already exists
	err = s.fs.MkDir(dirPath)

	s.NoError(err)
}

func (s *FsSvcTestSuite) TestMkDir_InvalidPermissions() {
	// This test is tricky - would need to create a directory with no write permissions
	// Skip for now as it's platform-dependent
}

func (s *FsSvcTestSuite) TestMkDir_WithFile() {
	// Create a file
	filePath := filepath.Join(s.tmpDir, "file.txt")
	err := os.WriteFile(filePath, []byte("test"), 0644)
	s.Require().NoError(err)

	// Try to create directory with same name
	err = s.fs.MkDir(filePath)

	// Should error because file exists
	s.Error(err)
}

func (s *FsSvcTestSuite) TestNewFsSvc() {
	fs := NewFsSvc(s.logger)

	s.NotNil(fs)
	s.IsType(&FsSvc{}, fs)

	// Verify it implements FileSystem interface
	var _ FileSystem = fs
}
