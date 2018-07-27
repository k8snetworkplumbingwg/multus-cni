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
//

package types

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConf(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "conf")
}

var _ = Describe("config operations", func() {
	It("parses a valid multus configuration", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net"
    }]
}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("weave-net"))
		Expect(netConf.Delegates[0].MasterPlugin).To(BeTrue())
	})

	It("succeeds if only delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "delegates": [{
        "type": "weave-net"
    },{
        "type": "foobar"
    }]
}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(2))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("weave-net"))
		Expect(netConf.Delegates[0].MasterPlugin).To(BeTrue())
		Expect(netConf.Delegates[1].Conf.Type).To(Equal("foobar"))
		Expect(netConf.Delegates[1].MasterPlugin).To(BeFalse())
	})

	It("fails if no kubeconfig or delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus"
}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
	})

	It("fails if kubeconfig is present but no delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml"
}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
	})
})
