// Copyright 2018 Google Inc. All Rights Reserved.
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

package ctpolicy

import (
	"github.com/google/certificate-transparency-go/loglist"
	"github.com/google/certificate-transparency-go/x509"
)

// AppleCTPolicy implements logic for complying with Apple's CT log policy.
type AppleCTPolicy struct{}

// LogsByGroup describes submission requirements for embedded SCTs according to https://support.apple.com/en-us/HT205280. Returns data even when error emitted.
func (appleP *AppleCTPolicy) LogsByGroup(cert *x509.Certificate, approved *loglist.LogList) (map[string]*LogGroupInfo, error) {
	var incCount int
	switch m := lifetimeInMonths(cert); {
	case m < 15:
		incCount = 2
	case m <= 27:
		incCount = 3
	case m <= 39:
		incCount = 4
	default:
		incCount = 5
	}
	baseGroup, err := baseGroupFor(approved, incCount)
	groups := map[string]*LogGroupInfo{baseGroup.name: &baseGroup}
	return groups, err
}
