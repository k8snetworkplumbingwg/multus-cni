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

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("thin entrypoint testing", func() {
	It("Run CopyFileAtomic()", func() {
		// create directory and files
		tmpDir, err := os.MkdirTemp("", "multus_thin_entrypoint_tmp")
		Expect(err).NotTo(HaveOccurred())

		// create source directory
		srcDir := fmt.Sprintf("%s/src", tmpDir)
		err = os.Mkdir(srcDir, 0755)
		Expect(err).NotTo(HaveOccurred())

		// create destination directory
		destDir := fmt.Sprintf("%s/dest", tmpDir)
		err = os.Mkdir(destDir, 0755)
		Expect(err).NotTo(HaveOccurred())

		// sample source file
		srcFilePath := fmt.Sprintf("%s/sampleInput", srcDir)
		err = os.WriteFile(srcFilePath, []byte("sampleInputABC"), 0744)
		Expect(err).NotTo(HaveOccurred())

		// old files in dest
		destFileName := "sampleInputDest"
		destFilePath := fmt.Sprintf("%s/%s", destDir, destFileName)
		err = os.WriteFile(destFilePath, []byte("inputOldXYZ"), 0611)
		Expect(err).NotTo(HaveOccurred())

		tempFileName := "temp_file"
		err = CopyFileAtomic(srcFilePath, destDir, tempFileName, destFileName)
		Expect(err).NotTo(HaveOccurred())

		// check file mode
		stat, err := os.Stat(destFilePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(stat.Mode()).To(Equal(os.FileMode(0744)))

		// check file contents
		destFileByte, err := os.ReadFile(destFilePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(destFileByte).To(Equal([]byte("sampleInputABC")))

		err = os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("opens cleaned absolute file paths as an os.Root and local file name", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_file_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		rawPath := tmpDir + string(os.PathSeparator) + "." + string(os.PathSeparator) + "sample"
		rootedFile, err := NewRootedFile(rawPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(rootedFile.Close()).To(Succeed())
		})

		Expect(rootedFile.FileName).To(Equal("sample"))
		Expect(rootedFile.Path()).To(Equal(filepath.Join(tmpDir, "sample")))

		file, err := rootedFile.Root.OpenFile(rootedFile.FileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte("rooted"))
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		contents, err := os.ReadFile(filepath.Join(tmpDir, "sample"))
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal([]byte("rooted")))
	})

	It("rejects unsafe rooted file paths", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_file_reject_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		unsafePaths := []string{
			"",
			"relative.conf",
			tmpDir + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "config",
		}
		for _, unsafePath := range unsafePaths {
			_, err := NewRootedFile(unsafePath)
			Expect(err).To(HaveOccurred())
		}
	})

	It("opens cleaned absolute directories with local file names", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_dir_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		rootedFile, err := NewRootedFileInDir(tmpDir+string(os.PathSeparator)+".", "dest")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(rootedFile.Close()).To(Succeed())
		})

		Expect(rootedFile.FileName).To(Equal("dest"))
		Expect(rootedFile.Path()).To(Equal(filepath.Join(tmpDir, "dest")))

		file, err := rootedFile.Root.Create(rootedFile.FileName)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte("dest"))
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		contents, err := os.ReadFile(filepath.Join(tmpDir, "dest"))
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal([]byte("dest")))
	})

	It("opens cleaned absolute directories as an os.Root", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_dir_root_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		rootedDir, err := NewRootedDir(tmpDir + string(os.PathSeparator) + ".")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(rootedDir.Close()).To(Succeed())
		})

		Expect(rootedDir.Path()).To(Equal(tmpDir))

		stat, err := rootedDir.Root.Stat(".")
		Expect(err).NotTo(HaveOccurred())
		Expect(stat.IsDir()).To(BeTrue())

		file, err := rootedDir.Root.Create("sample")
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte("root"))
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		contents, err := os.ReadFile(filepath.Join(tmpDir, "sample"))
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal([]byte("root")))
	})

	It("rejects unsafe rooted directory and file name combinations", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_dir_reject_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		_, err = NewRootedFileInDir("", "dest")
		Expect(err).To(HaveOccurred())

		_, err = NewRootedFileInDir("relative", "dest")
		Expect(err).To(HaveOccurred())

		_, err = NewRootedFileInDir(tmpDir+string(os.PathSeparator)+"..", "dest")
		Expect(err).To(HaveOccurred())

		_, err = NewRootedFileInDir(tmpDir, "../dest")
		Expect(err).To(HaveOccurred())

		_, err = NewRootedFileInDir(tmpDir, filepath.Join("nested", "dest"))
		Expect(err).To(HaveOccurred())

		_, err = NewRootedFileInDir(tmpDir, filepath.Join(tmpDir, "dest"))
		Expect(err).To(HaveOccurred())
	})

	It("rejects unsafe rooted directories", func() {
		tmpDir, err := os.MkdirTemp("", "multus_rooted_dir_only_reject_tmp")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		unsafeDirs := []string{
			"",
			"relative",
			tmpDir + string(os.PathSeparator) + "..",
		}
		for _, unsafeDir := range unsafeDirs {
			_, err := NewRootedDir(unsafeDir)
			Expect(err).To(HaveOccurred())
		}
	})
})
