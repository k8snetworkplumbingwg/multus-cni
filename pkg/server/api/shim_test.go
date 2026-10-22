// Copyright (c) 2022 Multus Authors
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

package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
)

// startShimTestServer starts a Unix HTTP server with the supplied handler and registers cleanup.
func startShimTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	socketDir := t.TempDir()
	listener, err := net.Listen("unix", SocketPath(socketDir))
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	server := &http.Server{Handler: handler}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("failed to close test server: %v", err)
		}
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("test server failed: %v", err)
		}
	})
	return socketDir
}

// TestCmdDel verifies communication errors, suppressed delegate errors, and daemon recovery.
func TestCmdDel(t *testing.T) {
	for _, scenario := range []string{"connection failure", "delegate failure", "daemon recovery"} {
		t.Run(scenario, func(t *testing.T) {
			var healthRequests, delRequests atomic.Int32
			socketDir := startShimTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case MultusHealthAPIEndpoint:
					if healthRequests.Add(1) == 1 && scenario == "daemon recovery" {
						http.Error(w, "starting", http.StatusServiceUnavailable)
					}
				case MultusCNIAPIEndpoint:
					delRequests.Add(1)
					if scenario == "connection failure" {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						conn.Close()
					} else if scenario == "delegate failure" {
						http.Error(w, "delegate cleanup failed", http.StatusBadRequest)
					}
				default:
					t.Errorf("unexpected endpoint %q", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})

			args := &skel.CmdArgs{StdinData: []byte(fmt.Sprintf(`{"cniVersion":"0.4.0","daemonSocketDir":%q}`, socketDir))}
			err := CmdDel(args)
			if scenario == "connection failure" {
				if err != errDaemonUnavailable {
					t.Fatalf("expected daemon communication error, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected DEL error: %v", err)
			}
			if delRequests.Load() != 1 {
				t.Fatalf("expected one DEL request, got %d", delRequests.Load())
			}
			if scenario == "daemon recovery" && healthRequests.Load() != 2 {
				t.Fatalf("expected a health retry, got %d requests", healthRequests.Load())
			}
		})
	}
}
