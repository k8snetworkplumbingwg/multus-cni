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

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
)

// ChrootExec implements invoke.Exec to execute CNI with chroot
type ChrootExec struct {
	Stderr    io.Writer
	chrootDir string
	version.PluginDecoder
}

var _ invoke.Exec = &ChrootExec{}

// ExecPlugin executes CNI plugin with given environment/stdin data.
func (e *ChrootExec) ExecPlugin(ctx context.Context, pluginPath string, stdinData []byte, environ []string) ([]byte, error) {
	var err error

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := exec.CommandContext(ctx, pluginPath)
	// execute delegate CNI with host filesystem context.
	c.SysProcAttr = &syscall.SysProcAttr{
		Chroot: e.chrootDir,
	}
	c.Env = environ
	c.Stdin = bytes.NewBuffer(stdinData)
	c.Stdout = stdout
	c.Stderr = stderr

	// Retry the command on "text file busy" errors
	for i := 0; i <= 5; i++ {
		err = c.Run()

		// Command succeeded
		if err == nil {
			break
		}

		// If the plugin is currently about to be written, then we wait a
		// second and try it again
		if strings.Contains(err.Error(), "text file busy") {
			time.Sleep(time.Second)
			continue
		}

		// All other errors except than the busy text file
		return nil, e.pluginErr(err, stdout.Bytes(), stderr.Bytes())
	}

	// Copy stderr to caller's buffer in case plugin printed to both
	// stdout and stderr for some reason. Ignore failures as stderr is
	// only informational.
	if e.Stderr != nil && stderr.Len() > 0 {
		_, _ = stderr.WriteTo(e.Stderr)
	}
	return stdout.Bytes(), nil
}

func (e *ChrootExec) pluginErr(err error, stdout, stderr []byte) error {
	emsg := types.Error{}
	if len(stdout) == 0 {
		if len(stderr) == 0 {
			emsg.Msg = fmt.Sprintf("netplugin failed with no error message: %v", err)
		} else {
			emsg.Msg = fmt.Sprintf("netplugin failed: %q", string(stderr))
		}
	} else if perr := json.Unmarshal(stdout, &emsg); perr != nil {
		emsg.Msg = fmt.Sprintf("netplugin failed but error parsing its diagnostic message %q: %v", string(stdout), perr)
	}
	return &emsg
}

// FindInPath try to find CNI plugin based on given path
func (e *ChrootExec) FindInPath(plugin string, paths []string) (string, error) {
	return invoke.FindInPath(plugin, paths)
}
