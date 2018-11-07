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

func wantedGroups(goog int, nonGoog int, base int, minusBob bool) map[string]*LogGroupInfo {
	gi := map[string]*LogGroupInfo{
		"Google-operated": {
			name: "Google-operated",
			LogURLs: map[string]bool{
				"ct.googleapis.com/aviator/":   true,
				"ct.googleapis.com/icarus/":    true,
				"ct.googleapis.com/rocketeer/": true,
				"ct.googleapis.com/racketeer/": true,
			},
			minInclusions: goog,
			isBase:        false,
		},
		"Non-Google-operated": {
			name: "Non-Google-operated",
			LogURLs: map[string]bool{
				"log.bob.io": true,
			},
			minInclusions: nonGoog,
			isBase:        false,
		},
		"All-logs": {
			name: "All-logs",
			LogURLs: map[string]bool{
				"ct.googleapis.com/aviator/":   true,
				"ct.googleapis.com/icarus/":    true,
				"ct.googleapis.com/rocketeer/": true,
				"ct.googleapis.com/racketeer/": true,
				"log.bob.io":                   true,
			},
			minInclusions: base,
			isBase:        true,
		},
	}
	if minusBob {
		delete(gi["All-logs"].LogURLs, "log.bob.io")
		delete(gi["Non-Google-operated"].LogURLs, "log.bob.io")
	}
	return gi
}

func TestCheckChromePolicy(t *testing.T) {
	tests := []struct {
		name string
		cert *x509.Certificate
		want map[string]*LogGroupInfo
	}{
		{
			name: "Short",
			cert: getTestCertPEMShort(),
			want: wantedGroups(1, 1, 2, false),
		},
		{
			name: "2-year",
			cert: getTestCertPEM2Years(),
			want: wantedGroups(1, 1, 3, false),
		},
		{
			name: "3-year",
			cert: getTestCertPEM3Years(),
			want: wantedGroups(1, 1, 4, false),
		},
		{
			name: "Long",
			cert: getTestCertPEMLongOriginal(),
			want: wantedGroups(1, 1, 5, false),
		},
	}

	var policy ChromeCTPolicy
	sampleLogList := sampleLogList(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := policy.LogsByGroup(test.cert, sampleLogList)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("LogsByGroup returned %v, want %v", got, test.want)
			}
			if err != nil {
				t.Errorf("LogsByGroup returned an error when not expected: %v", err)
			}
		})
	}
}

func TestCheckChromePolicyWarnings(t *testing.T) {
	tests := []struct {
		name    string
		cert    *x509.Certificate
		want    map[string]*LogGroupInfo
		warning string
	}{
		{
			name:    "Short",
			cert:    getTestCertPEMShort(),
			want:    wantedGroups(1, 1, 2, true),
			warning: "trying to assign 1 minimal inclusion number while only 0 logs are part of group \"Non-Google-operated\"",
		},
		{
			name:    "2-year",
			cert:    getTestCertPEM2Years(),
			want:    wantedGroups(1, 1, 3, true),
			warning: "trying to assign 1 minimal inclusion number while only 0 logs are part of group \"Non-Google-operated\"",
		},
		{
			name:    "3-year",
			cert:    getTestCertPEM3Years(),
			want:    wantedGroups(1, 1, 4, true),
			warning: "trying to assign 1 minimal inclusion number while only 0 logs are part of group \"Non-Google-operated\"",
		},
		{
			name:    "Long",
			cert:    getTestCertPEMLongOriginal(),
			want:    wantedGroups(1, 1, 5, true),
			warning: "trying to assign 5 minimal inclusion number while only 4 logs are part of group \"All-logs\"",
		},
	}

	var policy ChromeCTPolicy
	sampleLogList := sampleLogList(t)
	// Removing Bob-log.
	sampleLogList.Logs = sampleLogList.Logs[:4]

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := policy.LogsByGroup(test.cert, sampleLogList)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("LogsByGroup returned %v, want %v", got, test.want)
			}
			if err == nil && len(test.warning) > 0 {
				t.Errorf("LogsByGroup returned no error when expected")
			} else if err != nil {
				if err.Error() != test.warning {
					t.Errorf("LogsByGroup returned error message %q while expected %q", err.Error(), test.warning)
				}
			}
		})
	}
}
