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
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/cmdutils"
)

func TestWriteKubeconfig(t *testing.T) {
	tmpDir := t.TempDir()
	kubeconfigFile := filepath.Join(tmpDir, "kubeconfig")

	kubeconfigPath, err := cmdutils.NewRootedFile(kubeconfigFile)
	if err != nil {
		t.Fatalf("failed to create rooted kubeconfig path: %v", err)
	}
	defer kubeconfigPath.Close()

	templateData := map[string]string{
		"CADATA":        "test-ca",
		"CERTDIR":       tmpDir,
		"K8S_APISERVER": "https://api.example.test",
	}
	if err := writeKubeconfig(kubeconfigPath, templateData); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}

	contents, err := kubeconfigPath.Root.ReadFile(kubeconfigPath.FileName)
	if err != nil {
		t.Fatalf("failed to read kubeconfig: %v", err)
	}
	for _, expected := range []string{
		"certificate-authority-data: test-ca",
		"server: https://api.example.test",
		"client-certificate: " + tmpDir + "/multus-client-current.pem",
		"client-key: " + tmpDir + "/multus-client-current.pem",
	} {
		if !strings.Contains(string(contents), expected) {
			t.Fatalf("expected generated kubeconfig to contain %q, got:\n%s", expected, string(contents))
		}
	}

	stat, err := kubeconfigPath.Root.Stat(kubeconfigPath.FileName)
	if err != nil {
		t.Fatalf("failed to stat kubeconfig: %v", err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("expected kubeconfig mode 0600, got %v", stat.Mode().Perm())
	}
}
