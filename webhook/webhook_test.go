// Copyright (c) 2017 Intel Corporation
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

package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

  "bytes"
  "net/http"
  "net/http/httptest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/api/admission/v1beta1"

	"github.com/intel/multus-cni/types"
)

var _ = Describe("Webhook", func() {

  Describe("Preparing Admission Review Response", func() {
    Context("Admission Review Request is nil", func() {
      It("should return error", func() {
        ar := &v1beta1.AdmissionReview{}
        ar.Request = nil
        Expect(prepareAdmissionReviewResponse(false, "", ar)).To(HaveOccurred())
      })
    })
    Context("Message is not empty", func() {
      It("should set message in the response", func() {
        ar := &v1beta1.AdmissionReview{}
        ar.Request = &v1beta1.AdmissionRequest{
          UID: "fake-uid",
        }
        err := prepareAdmissionReviewResponse(false, "some message", ar)
        Expect(err).NotTo(HaveOccurred())
        Expect(ar.Response.Result.Message).To(Equal("some message"))
      })
    })
  })

  Describe("Deserializing Admission Review", func() {
    Context("It's not an Admission Review", func() {
      It("should return an error", func() {
        body := []byte("some-invalid-body")
        _, err := deserializeAdmissionReview(body)
        Expect(err).To(HaveOccurred())
      })
    })
  })

  Describe("Deserializing Network Attachment Definition", func() {
    Context("It's not an Network Attachment Definition", func() {
      It("should return an error", func() {
        ar := &v1beta1.AdmissionReview{}
        ar.Request = &v1beta1.AdmissionRequest{}
        _, err := deserializeNetworkAttachmentDefinition(*ar)
        Expect(err).To(HaveOccurred())
      })
    })
  })

  Describe("Handling validation request", func() {
    Context("Request body is empty", func() {
      It("should return an error", func() {
        req := httptest.NewRequest("POST", "https://fakewebhook/validate", nil)
        w := httptest.NewRecorder()
        validateHandler(w, req)
        resp := w.Result()
        Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
      })
    })

    Context("Content type is not application/json", func() {
      It("should return an error", func() {
        req := httptest.NewRequest("POST", "https://fakewebhook/validate", bytes.NewBufferString("fake-body"))
        req.Header.Set("Content-Type", "invalid-type")
        w := httptest.NewRecorder()
        validateHandler(w, req)
        resp := w.Result()
        Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType))
      })
    })
  })

	DescribeTable("Network Attachment Definition validation",
		func(in types.NetworkAttachmentDefinition, out bool, shouldFail bool) {
			actualOut, err := validateNetworkAttachmentDefinition(in)
			Expect(actualOut).To(Equal(out))
			if shouldFail {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry(
			"empty config",
			types.NetworkAttachmentDefinition{
				Metadata: metav1.ObjectMeta{
					Name: "some-valid-name",
				},
			},
			false, true,
		),
		Entry(
			"invalid name",
			types.NetworkAttachmentDefinition{
				Metadata: metav1.ObjectMeta{
					Name: "some_invalid_name",
				},
			},
			false, true,
		),
		Entry(
			"invalid network config",
			types.NetworkAttachmentDefinition{
				Metadata: metav1.ObjectMeta{
					Name: "some-valid-name",
				},
				Spec: types.NetworkAttachmentDefinitionSpec{
					Config: `{"some-invalid": "config"}`,
				},
			},
			false, true,
		),
		Entry(
			"valid network config",
			types.NetworkAttachmentDefinition{
				Metadata: metav1.ObjectMeta{
					Name: "some-valid-name",
				},
				Spec: types.NetworkAttachmentDefinitionSpec{
					Config: `{
            "cniVersion": "0.3.0",
            "type": "some-plugin"
          }`,
				},
			},
			true, false,
		),
		Entry(
			"valid network config list",
			types.NetworkAttachmentDefinition{
				Metadata: metav1.ObjectMeta{
					Name: "some-valid-name",
				},
				Spec: types.NetworkAttachmentDefinitionSpec{
					Config: `{
            "cniVersion": "0.3.0",
            "name": "some-bridge-network",
            "plugins": [
              {
                "type": "bridge",
                "bridge": "br0",
                "ipam": {
                  "type": "host-local",
                  "subnet": "192.168.1.0/24"
                }
              },
              {
                "type": "some-plugin"
              },
              {
                "type": "another-plugin",
                "sysctl": {
                  "net.ipv4.conf.all.log_martians": "1"
                }
              }
            ]
          }`,
				},
			},
			true, false,
		),
	)
})
