// Copyright (c) 2023 Multus Authors
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

package k8sclient

import (
	"fmt"
	"testing"

	"k8s.io/client-go/util/certificate"
)

func TestIsTransientCertError(t *testing.T) {
	noCertKeyErr := certificate.NoCertKeyError(`no cert/key files read at "/var/lib/multus"`)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "NoCertKeyError",
			err:      &noCertKeyErr,
			expected: false,
		},
		{
			name:     "file not found during rotation",
			err:      fmt.Errorf("could not convert data from \"/certs/current.pem\" into cert/key pair: open /certs/current.pem: no such file or directory"),
			expected: true,
		},
		{
			name:     "empty PEM file during rotation",
			err:      fmt.Errorf("could not convert data from \"/certs/current.pem\" into cert/key pair: failed to find any PEM data in certificate input"),
			expected: true,
		},
		{
			name:     "invalid PEM data during rotation",
			err:      fmt.Errorf("invalid PEM block"),
			expected: true,
		},
		{
			name:     "unrecognized error is not transient",
			err:      fmt.Errorf("some unknown certificate error"),
			expected: false,
		},
		{
			name:     "permission denied is not transient",
			err:      fmt.Errorf("permission denied"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientCertError(tt.err)
			if got != tt.expected {
				t.Errorf("isTransientCertError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
