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

package server

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/testutils"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/api"
)

// Observe first-plugin completion and the end of ADD rollback.
type cancellationTestExec struct {
	invoke.Exec
	afterFirst   func(context.Context)
	rollbackDone chan struct{}
}

func (e *cancellationTestExec) ExecPlugin(ctx context.Context, path string, stdin []byte, env []string) ([]byte, error) {
	result, err := e.Exec.ExecPlugin(ctx, path, stdin, env)
	if filepath.Base(path) == "first" {
		switch cniCommand(env) {
		case "ADD":
			if err == nil && e.afterFirst != nil {
				e.afterFirst(ctx)
			}
		case "DEL":
			close(e.rollbackDone)
		}
	}
	return result, err
}

// Run the shim entry point in a child process that the test can terminate.
func TestShimProcess(_ *testing.T) {
	if os.Getenv("MULTUS_TEST_SHIM") != "1" {
		return
	}
	skel.PluginMain(api.CmdAdd, api.CmdCheck, api.CmdDel, version.All, "test shim")
	os.Exit(0)
}

var _ = Describe("Request cancellation", func() {
	It("cancels ADD and CHECK while allowing DEL cleanup", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, exec := range []invoke.Exec{nil, &invoke.DefaultExec{RawExec: &invoke.RawExec{}}, &ChrootExec{}} {
			request := newRequestExec(ctx, exec)
			path, err := request.FindInPath("true", []string{"/bin"})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("/bin/true"))
			for _, command := range []string{"ADD", "CHECK", "DEL"} {
				_, err := request.ExecPlugin(context.Background(), "/bin/true", nil, []string{"CNI_COMMAND=" + command})
				if command == "DEL" {
					Expect(err).NotTo(HaveOccurred(), "DEL cleanup must survive cancellation")
				} else {
					Expect(err).To(HaveOccurred(), "%s must use the canceled context", command)
				}
			}
		}
	})

	Context("shim disconnect", func() {
		var (
			server        *Server
			executor      *cancellationTestExec
			shim          *exec.Cmd
			dir           string
			firstPIDFile  string
			secondADDFile string
		)

		writeFirstPlugin := func(command string) {
			GinkgoHelper()
			script := fmt.Sprintf(`#!/bin/sh
if [ "$CNI_COMMAND" = ADD ]; then
    echo $$ > %q
    %s
fi
echo '{"cniVersion":"0.4.0"}'
`, firstPIDFile, command)
			Expect(os.WriteFile(filepath.Join(dir, "first"), []byte(script), 0700)).To(Succeed())
		}

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
			firstPIDFile = filepath.Join(dir, "first.pid")
			secondADDFile = filepath.Join(dir, "second.add")
			writeFirstPlugin("exec sleep 300")
			secondScript := fmt.Sprintf(`#!/bin/sh
[ "$CNI_COMMAND" != ADD ] || touch %q
echo '{"cniVersion":"0.4.0"}'
`, secondADDFile)
			Expect(os.WriteFile(filepath.Join(dir, "second"), []byte(secondScript), 0700)).To(Succeed())

			client := fakeK8sClient()
			ctx, cancel := context.WithCancel(context.Background())
			var err error
			server, err = startCNIServer(ctx, dir, client, nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				cancel()
				Expect(server.Close()).To(Succeed())
				unregisterMetrics(server)
			})
			netns, err := testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(netns.Close()).To(Succeed())
				Expect(syscall.Unmount(netns.Path(), syscall.MNT_DETACH)).To(Succeed())
				Expect(os.Remove(netns.Path())).To(Succeed())
			})
			Expect(prepareCNIEnv(netns.Path(), "test", "cancel-test", "testUID")).To(Succeed())
			DeferCleanup(teardownCNIEnv)
			Expect(createFakePod(client, "cancel-test")).To(Succeed())

			executor = &cancellationTestExec{
				Exec:         &invoke.DefaultExec{RawExec: &invoke.RawExec{}},
				rollbackDone: make(chan struct{}),
			}
			server.exec = executor
			config := fmt.Sprintf(`{
                "cniVersion": "0.4.0", "name": "cancel-test", "type": "multus-shim",
                "daemonSocketDir": %q, "binDir": %q, "cniDir": %q,
                "delegates": [
                    {"cniVersion": "0.4.0", "name": "first", "type": "first"},
                    {"cniVersion": "0.4.0", "name": "second", "type": "second"}
                ]
            }`, dir, dir, dir)
			shim = exec.Command(os.Args[0], "-test.run=^TestShimProcess$")
			shim.Env = append(os.Environ(), "MULTUS_TEST_SHIM=1", "CNI_COMMAND=ADD", "CNI_IFNAME=eth0", "CNI_PATH="+dir)
			shim.Stdin = bytes.NewBufferString(config)
			shim.Stdout, shim.Stderr = GinkgoWriter, GinkgoWriter
		})

		startShim := func() int {
			GinkgoHelper()
			Expect(shim.Start()).To(Succeed())
			DeferCleanup(func() {
				_ = shim.Process.Kill()
				_ = shim.Wait()
			})
			var pid int
			Eventually(func() int {
				data, _ := os.ReadFile(firstPIDFile)
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				return pid
			}, "10s").Should(BeNumerically(">", 0))
			DeferCleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			return pid
		}

		expectCanceled := func(firstPID int) {
			GinkgoHelper()
			Eventually(executor.rollbackDone, "10s").Should(BeClosed())
			Expect(syscall.Kill(firstPID, 0)).To(Equal(syscall.ESRCH), "first plugin must have exited")
			Expect(secondADDFile).NotTo(BeAnExistingFile(), "second plugin ADD must never start")
		}

		It("kills the blocked first plugin and never starts the second ADD", func() {
			firstPID := startShim()

			Expect(shim.Process.Kill()).To(Succeed())
			Expect(shim.Wait()).To(HaveOccurred())

			expectCanceled(firstPID)
		})

		It("never starts the second ADD when cancellation happens between plugins", func() {
			writeFirstPlugin("true") // Return a successful CNI result immediately.
			paused := make(chan context.Context, 1)
			resume := make(chan struct{}, 1)
			DeferCleanup(func() { close(resume) })
			executor.afterFirst = func(ctx context.Context) {
				paused <- ctx
				<-resume
			}
			firstPID := startShim()
			var requestCtx context.Context
			Eventually(paused, "10s").Should(Receive(&requestCtx))

			Expect(shim.Process.Kill()).To(Succeed())
			Expect(shim.Wait()).To(HaveOccurred())
			Eventually(requestCtx.Done(), "10s").Should(BeClosed())
			resume <- struct{}{}

			expectCanceled(firstPID)
		})
	})
})
