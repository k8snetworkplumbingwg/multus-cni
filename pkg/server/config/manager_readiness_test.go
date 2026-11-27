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

package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestManagerReadinessShutdown(t *testing.T) {
	if os.Getenv("MULTUS_TEST_READINESS_CHILD") == "1" {
		runReadinessShutdown(t)
		return
	}

	for _, policy := range []string{"default", "enabled", "disabled"} {
		for _, event := range []string{"remove", "rename"} {
			t.Run(policy+"/"+event, func(t *testing.T) {
				dir := t.TempDir()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestManagerReadinessShutdown$")
				cmd.Env = append(os.Environ(), "MULTUS_TEST_READINESS_CHILD=1",
					"MULTUS_TEST_CONFIG_DIR="+dir, "MULTUS_TEST_CLEANUP="+policy,
					"MULTUS_TEST_READINESS_EVENT="+event)
				output, err := cmd.CombinedOutput()
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
					t.Fatalf("readiness %s should exit with code 2, got %v: %s", event, err, output)
				}
				configPath := filepath.Join(dir, multusConfigFileName)
				_, err = os.Stat(configPath)
				if policy == "disabled" {
					if err != nil {
						t.Fatalf("disabled cleanup should preserve generated config %q on readiness %s: %v", configPath, event, err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("%s cleanup should remove generated config %q on readiness %s, stat error: %v", policy, configPath, event, err)
				}
			})
		}
	}
}

func runReadinessShutdown(t *testing.T) {
	t.Helper()
	dir := os.Getenv("MULTUS_TEST_CONFIG_DIR")
	primaryConfig := filepath.Join(dir, "10-primary.conf")
	if err := os.WriteFile(primaryConfig, []byte(`{"cniVersion":"0.4.0","name":"primary","type":"mycni"}`), UserRWPermission); err != nil {
		t.Fatal(err)
	}
	readinessFile := filepath.Join(dir, "ready")
	if err := os.WriteFile(readinessFile, nil, UserRWPermission); err != nil {
		t.Fatal(err)
	}
	conf := MultusConf{
		CNIVersion: "0.4.0", Name: MultusDefaultNetworkName,
		CniConfigDir: dir, MultusAutoconfigDir: dir, MultusMasterCni: "10-primary.conf",
		ReadinessIndicatorFile: readinessFile,
	}
	if policy := os.Getenv("MULTUS_TEST_CLEANUP"); policy != "default" {
		cleanup := policy == "enabled"
		conf.CleanupConfigOnExit = &cleanup
	}
	manager, err := NewManager(conf)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx, &sync.WaitGroup{}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("MULTUS_TEST_READINESS_EVENT") == "rename" {
		err = os.Rename(readinessFile, readinessFile+".old")
	} else {
		err = os.Remove(readinessFile)
	}
	if err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
}
