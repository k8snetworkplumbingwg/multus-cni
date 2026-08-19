// Copyright (c) 2026 Multus Authors
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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/cmdutils"
)

func TestKubeconfigGenerator(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "kubeconfig_generator")
}

var _ = ginkgo.Describe("kubeconfig generator", func() {
	ginkgo.It("writes a new kubeconfig with private file mode", func() {
		tmpDir, err := os.MkdirTemp("", "multus_kubeconfig_generator_tmp")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(os.RemoveAll(tmpDir)).To(gomega.Succeed())
		})

		kubeconfigPath, err := cmdutils.NewRootedFile(filepath.Join(tmpDir, "kubeconfig"))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(kubeconfigPath.Close()).To(gomega.Succeed())
		})

		gomega.Expect(writeKubeconfig(kubeconfigPath, templateData(tmpDir))).To(gomega.Succeed())

		expectKubeconfigContents(kubeconfigPath, tmpDir)
		stat, err := kubeconfigPath.Root.Stat(kubeconfigPath.FileName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(stat.Mode().Perm()).To(gomega.Equal(os.FileMode(0600)))
	})

	ginkgo.It("writes kubeconfig and resets an existing permissive file mode", func() {
		tmpDir, err := os.MkdirTemp("", "multus_kubeconfig_generator_tmp")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(os.RemoveAll(tmpDir)).To(gomega.Succeed())
		})

		kubeconfigPath, err := cmdutils.NewRootedFile(filepath.Join(tmpDir, "kubeconfig"))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			gomega.Expect(kubeconfigPath.Close()).To(gomega.Succeed())
		})

		createExistingKubeconfig(kubeconfigPath)
		stat, err := kubeconfigPath.Root.Stat(kubeconfigPath.FileName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(stat.Mode().Perm()).To(gomega.Equal(os.FileMode(0644)))

		gomega.Expect(writeKubeconfig(kubeconfigPath, templateData(tmpDir))).To(gomega.Succeed())

		expectKubeconfigContents(kubeconfigPath, tmpDir)
		stat, err = kubeconfigPath.Root.Stat(kubeconfigPath.FileName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(stat.Mode().Perm()).To(gomega.Equal(os.FileMode(0600)))
	})
})

func templateData(certDir string) map[string]string {
	return map[string]string{
		"CADATA":        "test-ca",
		"CERTDIR":       certDir,
		"K8S_APISERVER": "https://api.example.test",
	}
}

func expectKubeconfigContents(kubeconfigPath *cmdutils.RootedFile, certDir string) {
	contents, err := kubeconfigPath.Root.ReadFile(kubeconfigPath.FileName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for _, expected := range []string{
		"certificate-authority-data: test-ca",
		"server: https://api.example.test",
		"client-certificate: " + certDir + "/multus-client-current.pem",
		"client-key: " + certDir + "/multus-client-current.pem",
	} {
		gomega.Expect(string(contents)).To(gomega.ContainSubstring(expected))
	}
}

func createExistingKubeconfig(kubeconfigPath *cmdutils.RootedFile) {
	existingFile, err := kubeconfigPath.Root.OpenFile(kubeconfigPath.FileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		gomega.Expect(existingFile.Close()).To(gomega.Succeed())
	}()

	_, err = existingFile.Write([]byte("existing"))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(existingFile.Chmod(0644)).To(gomega.Succeed())
}
