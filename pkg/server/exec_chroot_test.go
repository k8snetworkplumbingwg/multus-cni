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

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("exec_chroot", func() {

	It("Call ChrootExec.ExecPlugin with dummy", func() {
		chrootExec := &ChrootExec{
			Stderr:    os.Stderr,
			chrootDir: "/usr",
		}

		_, err := chrootExec.ExecPlugin(context.Background(), "/bin/true", nil, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Call invalid ChrootExec.ExecPlugin with dummy", func() {
		chrootExec := &ChrootExec{
			Stderr:    os.Stderr,
			chrootDir: "/tmp",
		}

		_, err := chrootExec.ExecPlugin(context.Background(), "/bin/true", nil, nil)
		Expect(err).To(HaveOccurred())
	})
})
