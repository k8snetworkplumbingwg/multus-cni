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

package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/intel/multus-cni/types"
	. "github.com/intel/multus-cni/webhook"
)

var _ = Describe("Webhook", func() {
	DescribeTable("Network Attachment Definition validation",
		func(in types.NetworkAttachmentDefinition, out bool, shouldFail bool) {
			actualOut, err := ValidateNetworkAttachmentDefinition(in)
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
