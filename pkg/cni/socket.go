// Copyright (c) 2021 Multus Authors
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
//

package cni

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	serverSocketName                   = "multus-cni.sock"
	fullReadWriteExecutePermissions    = 0777
	thickPluginSocketRunDirPermissions = 0700
)

// FilesystemPreRequirements ensures the target `rundir` features the correct
// permissions.
func FilesystemPreRequirements(rundir string) error {
	socketpath := SocketPath(rundir)
	if err := os.RemoveAll(rundir); err != nil && !os.IsNotExist(err) {
		info, err := os.Stat(rundir)
		if err != nil {
			return fmt.Errorf("failed to stat old pod info socket directory %s: %v", rundir, err)
		}
		// Owner must be root
		tmp := info.Sys()
		statt, ok := tmp.(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to read pod info socket directory stat info: %T", tmp)
		}
		if statt.Uid != 0 {
			return fmt.Errorf("insecure owner of pod info socket directory %s: %v", rundir, statt.Uid)
		}

		// Check permissions
		if info.Mode()&fullReadWriteExecutePermissions != thickPluginSocketRunDirPermissions {
			return fmt.Errorf("insecure permissions on pod info socket directory %s: %v", rundir, info.Mode())
		}

		// Finally remove the socket file so we can re-create it
		if err := os.Remove(socketpath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove old pod info socket %s: %v", socketpath, err)
		}
	}
	if err := os.MkdirAll(rundir, thickPluginSocketRunDirPermissions); err != nil {
		return fmt.Errorf("failed to create pod info socket directory %s: %v", rundir, err)
	}
	return nil
}

// SocketPath returns the path of the multus CNI socket
func SocketPath(rundir string) string {
	return filepath.Join(rundir, serverSocketName)
}
