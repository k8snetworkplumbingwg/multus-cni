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
	"reflect"
	"testing"

	"github.com/google/certificate-transparency-go/x509"
)

func wantedAppleGroups(count int) map[string]*LogGroupInfo {
	gi := map[string]*LogGroupInfo{
		"All-logs": {
			name: "All-logs",
			LogURLs: map[string]bool{
				"ct.googleapis.com/aviator/":   true,
				"ct.googleapis.com/icarus/":    true,
				"ct.googleapis.com/rocketeer/": true,
				"ct.googleapis.com/racketeer/": true,
				"log.bob.io":                   true,
			},
			minInclusions: count,
			isBase:        true,
		},
	}
	return gi
}

func TestCheckApplePolicy(t *testing.T) {
	tests := []struct {
		name string
		cert *x509.Certificate
		want map[string]*LogGroupInfo
	}{
		{
			name: "Short",
			cert: getTestCertPEMShort(),
			want: wantedAppleGroups(2),
		},
		{
			name: "2-year",
			cert: getTestCertPEM2Years(),
			want: wantedAppleGroups(3),
		},
		{
			name: "3-year",
			cert: getTestCertPEM3Years(),
			want: wantedAppleGroups(4),
		},
		{
			name: "Long",
			cert: getTestCertPEMLongOriginal(),
			want: wantedAppleGroups(5),
		},
	}

	var policy AppleCTPolicy
	sampleLogList := sampleLogList(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			groups, err := policy.LogsByGroup(test.cert, sampleLogList)
			if !reflect.DeepEqual(groups, test.want) {
				t.Errorf("LogsByGroup returned %v, want %v", groups, test.want)
			}
			if err != nil {
				t.Errorf("LogsByGroup returned an error: %v", err)
			}
		})
	}
}
