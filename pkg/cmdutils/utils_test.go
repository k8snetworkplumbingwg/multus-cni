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
	"os"

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
})
