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

// ChromeCTPolicy implements logic for complying with Chrome's CT log policy
type ChromeCTPolicy struct {
}

// LogsByGroup describes submission requirements for embedded SCTs according to https://github.com/chromium/ct-policy/blob/master/ct_policy.md#qualifying-certificate.
func (chromeP *ChromeCTPolicy) LogsByGroup(cert *x509.Certificate, approved *loglist.LogList) (map[string]*LogGroupInfo, error) {
	var outerror error
	googGroup := LogGroupInfo{name: "Google-operated", isBase: false}
	googGroup.populate(approved, func(log *loglist.Log) bool { return log.GoogleOperated() })
	if err := googGroup.setMinInclusions(1); err != nil {
		outerror = err
	}
	nonGoogGroup := LogGroupInfo{name: "Non-Google-operated", isBase: false}
	nonGoogGroup.populate(approved, func(log *loglist.Log) bool { return !log.GoogleOperated() })
	if err := nonGoogGroup.setMinInclusions(1); err != nil {
		outerror = err
	}

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
	if err != nil {
		outerror = err
	}
	groups := map[string]*LogGroupInfo{
		googGroup.name:    &googGroup,
		nonGoogGroup.name: &nonGoogGroup,
		baseGroup.name:    &baseGroup,
	}
	return groups, outerror
}
