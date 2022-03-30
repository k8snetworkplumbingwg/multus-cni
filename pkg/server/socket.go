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

package server

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	serverSocketName                   = "multus.sock"
	fullReadWriteExecutePermissions    = 0777
	thickPluginSocketRunDirPermissions = 0700
)

// FilesystemPreRequirements ensures the target `rundir` features the correct
// permissions.
func FilesystemPreRequirements(rundir string) error {
	if err := os.RemoveAll(rundir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old pod info socket directory %s: %v", rundir, err)
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
