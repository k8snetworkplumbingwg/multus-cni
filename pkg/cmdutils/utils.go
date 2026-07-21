// Copyright (c) 2023 Multus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package cmdutils is the package that contains utilities for multus command
package cmdutils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RootedFile is a validated file path split into an opened directory root and
// a local file name for os.Root filesystem operations.
type RootedFile struct {
	Root     *os.Root
	FileName string

	path string
}

// NewRootedFile validates rawPath as an absolute file path and opens its parent
// directory as a root.
func NewRootedFile(rawPath string) (*RootedFile, error) {
	path, err := cleanAbsolutePath(rawPath)
	if err != nil {
		return nil, err
	}
	fileName, err := cleanLocalFileName(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	return openRootedFile(filepath.Dir(path), fileName)
}

// NewRootedFileInDir validates rootDir and fileName separately before opening
// rootDir as a root.
func NewRootedFileInDir(rootDir, fileName string) (*RootedFile, error) {
	cleanRootDir, err := cleanAbsolutePath(rootDir)
	if err != nil {
		return nil, err
	}
	cleanFileName, err := cleanLocalFileName(fileName)
	if err != nil {
		return nil, err
	}
	return openRootedFile(cleanRootDir, cleanFileName)
}

// Path returns the cleaned absolute path for logging and error messages.
func (r *RootedFile) Path() string {
	return r.path
}

// Close closes the opened root directory.
func (r *RootedFile) Close() error {
	if r == nil || r.Root == nil {
		return nil
	}
	return r.Root.Close()
}

func openRootedFile(rootDir, fileName string) (*RootedFile, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open root directory %q: %w", rootDir, err)
	}
	return &RootedFile{
		Root:     root,
		FileName: fileName,
		path:     filepath.Join(rootDir, fileName),
	}, nil
}

func cleanAbsolutePath(rawPath string) (string, error) {
	if rawPath == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if hasParentDirComponent(rawPath) {
		return "", fmt.Errorf("path %q must not contain parent directory references", rawPath)
	}
	cleanPath := filepath.Clean(rawPath)
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("path %q must be absolute", rawPath)
	}
	return cleanPath, nil
}

func cleanLocalFileName(fileName string) (string, error) {
	if fileName == "" {
		return "", fmt.Errorf("file name must not be empty")
	}
	if hasParentDirComponent(fileName) {
		return "", fmt.Errorf("file name %q must not contain parent directory references", fileName)
	}
	cleanFileName := filepath.Clean(fileName)
	if cleanFileName == "." ||
		cleanFileName == string(os.PathSeparator) ||
		filepath.IsAbs(cleanFileName) ||
		cleanFileName != filepath.Base(cleanFileName) {
		return "", fmt.Errorf("file name %q must be local", fileName)
	}
	return cleanFileName, nil
}

func hasParentDirComponent(path string) bool {
	for _, component := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return true
		}
	}
	return false
}

// CopyFileAtomic does file copy atomically
func CopyFileAtomic(srcFilePath, destDir, tempFileName, destFileName string) error {
	tempFilePath := filepath.Join(destDir, tempFileName)
	// check temp filepath and remove old file if exists
	if _, err := os.Stat(tempFilePath); err == nil {
		err = os.Remove(tempFilePath)
		if err != nil {
			return fmt.Errorf("cannot remove old temp file %q: %v", tempFilePath, err)
		}
	}

	// create temp file
	f, err := os.CreateTemp(destDir, tempFileName)
	defer f.Close()
	if err != nil {
		return fmt.Errorf("cannot create temp file %q in %q: %v", tempFileName, destDir, err)
	}

	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %v", srcFilePath, err)
	}
	defer srcFile.Close()

	// Copy file to tempfile
	_, err = io.Copy(f, srcFile)
	if err != nil {
		f.Close()
		os.Remove(tempFilePath)
		return fmt.Errorf("cannot write data to temp file %q: %v", tempFilePath, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("cannot flush temp file %q: %v", tempFilePath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cannot close temp file %q: %v", tempFilePath, err)
	}

	// change file mode if different
	destFilePath := filepath.Join(destDir, destFileName)
	_, err = os.Stat(destFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	srcFileStat, err := os.Stat(srcFilePath)
	if err != nil {
		return err
	}

	if err := os.Chmod(f.Name(), srcFileStat.Mode()); err != nil {
		return fmt.Errorf("cannot set stat on temp file %q: %v", f.Name(), err)
	}

	// replace file with tempfile
	if err := os.Rename(f.Name(), destFilePath); err != nil {
		return fmt.Errorf("cannot replace %q with temp file %q: %v", destFilePath, tempFilePath, err)
	}

	return nil
}
