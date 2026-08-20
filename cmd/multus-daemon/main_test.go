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

	srv "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server"
)

func TestCNIServerConfigReadsValidatedAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "daemon-config.json")
	configBytes := []byte(`{"logFile":"/etc/cni/net.d/multus.d/daemon-config.json","socketDir":"/run/multus/"}`)
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatalf("failed to write test daemon config: %v", err)
	}

	config, err := cniServerConfig(configPath)
	if err != nil {
		t.Fatalf("expected daemon config to load: %v", err)
	}
	if config.SocketDir != "/run/multus/" {
		t.Fatalf("expected socketDir to load, got %q", config.SocketDir)
	}
	if string(config.ConfigFileContents) != string(configBytes) {
		t.Fatalf("expected config contents to be preserved")
	}
}

func TestCNIServerConfigRejectsUnsafePaths(t *testing.T) {
	tmpDir := t.TempDir()
	unsafePaths := []string{
		"",
		"daemon-config.json",
		tmpDir + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "daemon-config.json",
	}

	for _, unsafePath := range unsafePaths {
		if _, err := cniServerConfig(unsafePath); err == nil {
			t.Fatalf("expected cniServerConfig to reject %q", unsafePath)
		}
	}
}

func TestCopyUserProvidedConfigUsesValidatedRoots(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "multus.conf")
	if err := os.WriteFile(srcPath, []byte("multus-config"), 0644); err != nil {
		t.Fatalf("failed to write source config: %v", err)
	}

	if err := copyUserProvidedConfig(srcPath, destDir); err != nil {
		t.Fatalf("expected copyUserProvidedConfig to copy config: %v", err)
	}

	copiedConfig, err := os.ReadFile(filepath.Join(destDir, "multus.conf"))
	if err != nil {
		t.Fatalf("failed to read copied config: %v", err)
	}
	if string(copiedConfig) != "multus-config" {
		t.Fatalf("unexpected copied config: %q", copiedConfig)
	}
}

func TestCopyUserProvidedConfigRejectsUnsafePaths(t *testing.T) {
	tmpDir := t.TempDir()
	validSrcPath := filepath.Join(tmpDir, "multus.conf")
	if err := os.WriteFile(validSrcPath, []byte("multus-config"), 0644); err != nil {
		t.Fatalf("failed to write source config: %v", err)
	}

	testCases := []struct {
		name             string
		multusConfigPath string
		cniConfigDir     string
	}{
		{
			name:             "empty source",
			multusConfigPath: "",
			cniConfigDir:     tmpDir,
		},
		{
			name:             "relative source",
			multusConfigPath: "multus.conf",
			cniConfigDir:     tmpDir,
		},
		{
			name:             "parent source",
			multusConfigPath: tmpDir + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "multus.conf",
			cniConfigDir:     tmpDir,
		},
		{
			name:             "relative destination",
			multusConfigPath: validSrcPath,
			cniConfigDir:     "relative",
		},
		{
			name:             "parent destination",
			multusConfigPath: validSrcPath,
			cniConfigDir:     tmpDir + string(os.PathSeparator) + "..",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := copyUserProvidedConfig(tc.multusConfigPath, tc.cniConfigDir); err == nil {
				t.Fatalf("expected copyUserProvidedConfig to reject source %q and destination %q", tc.multusConfigPath, tc.cniConfigDir)
			}
		})
	}
}

func TestCNIServerConfigUsesDefaultSocketDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "daemon-config.json")
	configBytes := []byte(`{"logFile":"/etc/cni/net.d/multus.d/daemon-config.json"}`)
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatalf("failed to write test daemon config: %v", err)
	}

	config, err := cniServerConfig(configPath)
	if err != nil {
		t.Fatalf("expected daemon config to load: %v", err)
	}
	if config.SocketDir != srv.DefaultMultusRunDir {
		t.Fatalf("expected default socketDir %q, got %q", srv.DefaultMultusRunDir, config.SocketDir)
	}
}
