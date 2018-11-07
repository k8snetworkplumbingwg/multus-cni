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
	"encoding/json"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/loglist"
	"github.com/google/certificate-transparency-go/testdata"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"
)

func getTestCertPEMShort() *x509.Certificate {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestCertPEM))
	cert.NotAfter = time.Date(2013, 1, 1, 0, 0, 0, 0, time.UTC)
	return cert
}

func getTestCertPEM2Years() *x509.Certificate {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestCertPEM))
	cert.NotAfter = time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC)
	return cert
}

func getTestCertPEM3Years() *x509.Certificate {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestCertPEM))
	cert.NotAfter = time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	return cert
}

func getTestCertPEMLongOriginal() *x509.Certificate {
	cert, _ := x509util.CertificateFromPEM([]byte(testdata.TestCertPEM))
	return cert
}

func sampleLogList(t *testing.T) *loglist.LogList {
	t.Helper()
	var loglist loglist.LogList
	err := json.Unmarshal([]byte(testdata.SampleLogList), &loglist)
	if err != nil {
		t.Fatalf("Unable to Unmarshal testdata.SampleLogList %v", err)
	}
	return &loglist
}

func TestLifetimeInMonths(t *testing.T) {
	tests := []struct {
		name      string
		notBefore time.Time
		notAfter  time.Time
		want      int
	}{
		{
			name:      "ExactMonths",
			notBefore: time.Date(2012, 6, 1, 0, 0, 0, 0, time.UTC),
			notAfter:  time.Date(2013, 1, 1, 0, 0, 0, 0, time.UTC),
			want:      7,
		},
		{
			name:      "ExactYears",
			notBefore: time.Date(2012, 6, 1, 0, 0, 0, 0, time.UTC),
			notAfter:  time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
			want:      31,
		},
		{
			name:      "PartialSingleMonth",
			notBefore: time.Date(2012, 6, 1, 0, 0, 0, 0, time.UTC),
			notAfter:  time.Date(2012, 6, 25, 0, 0, 0, 0, time.UTC),
			want:      0,
		},
		{
			name:      "PartialMonths",
			notBefore: time.Date(2012, 6, 25, 0, 0, 0, 0, time.UTC),
			notAfter:  time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),
			want:      12,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cert := getTestCertPEMLongOriginal()
			cert.NotBefore = test.notBefore
			cert.NotAfter = test.notAfter
			got := lifetimeInMonths(cert)
			if got != test.want {
				t.Errorf("lifetimeInMonths(%v, %v)=%d, want %d", test.notBefore, test.notAfter, got, test.want)
			}
		})
	}
}
