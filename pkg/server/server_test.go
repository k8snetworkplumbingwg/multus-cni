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

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", func() {
	cniConf := []byte(`{
	"binDir": "/var/lib/cni/bin",
	"clusterNetwork": "/host/run/multus/cni/net.d/10-ovn-kubernetes.conf",
	"cniVersion": "0.3.1",
	"daemonSocketDir": "/run/multus/socket",
	"globalNamespaces": "default,openshift-multus,openshift-sriov-network-operator",
	"logLevel": "verbose",
	"logToStderr": true,
	"name": "multus-cni-network",
	"namespaceIsolation": true,
	"type": "multus-shim"
}`)

	serverConf := []byte(`{
	"cniVersion": "0.4.0",
	"chrootDir": "/hostroot",
	"logToStderr": false,
	"logLevel": "debug",
	"binDir": "/foo/bar",
	"cniConfigDir": "/host/etc/cni/net.d",
	"multusConfigFile": "auto",
	"multusAutoconfigDir": "/host/run/multus/cni/net.d",
	"namespaceIsolation": false,
	"globalNamespaces": "other,namespace",
	"readinessindicatorfile": "/host/run/multus/cni/net.d/10-ovn-kubernetes.conf",
	"daemonSocketDir": "/somewhere/socket",
	"socketDir": "/host/run/multus/socket"
}`)

	Context("correctly overrides incoming CNI config with server config", func() {
		newConf, err := overrideCNIConfigWithServerConfig(cniConf, serverConf, false)
		Expect(err).ToNot(HaveOccurred())

		// All server options except readinessindicatorfile should exist
		// in the returned config
		Expect(newConf).To(MatchJSON(`{
	"clusterNetwork": "/host/run/multus/cni/net.d/10-ovn-kubernetes.conf",
	"name": "multus-cni-network",
	"type": "multus-shim",
	"cniVersion": "0.4.0",
	"chrootDir": "/hostroot",
	"logToStderr": false,
	"logLevel": "debug",
	"binDir": "/foo/bar",
	"cniConfigDir": "/host/etc/cni/net.d",
	"multusConfigFile": "auto",
	"multusAutoconfigDir": "/host/run/multus/cni/net.d",
	"namespaceIsolation": false,
	"globalNamespaces": "other,namespace",
	"readinessindicatorfile": "/host/run/multus/cni/net.d/10-ovn-kubernetes.conf",
	"daemonSocketDir": "/somewhere/socket",
	"socketDir": "/host/run/multus/socket"
}`))
	})

	Context("correctly overrides incoming CNI config with server config and ignores readinessindicatorfile", func() {
		newConf, err := overrideCNIConfigWithServerConfig(cniConf, serverConf, true)
		Expect(err).ToNot(HaveOccurred())

		// All server options except readinessindicatorfile should exist
		// in the returned config
		Expect(newConf).To(MatchJSON(`{
	"clusterNetwork": "/host/run/multus/cni/net.d/10-ovn-kubernetes.conf",
	"name": "multus-cni-network",
	"type": "multus-shim",
	"cniVersion": "0.4.0",
	"chrootDir": "/hostroot",
	"logToStderr": false,
	"logLevel": "debug",
	"binDir": "/foo/bar",
	"cniConfigDir": "/host/etc/cni/net.d",
	"multusConfigFile": "auto",
	"multusAutoconfigDir": "/host/run/multus/cni/net.d",
	"namespaceIsolation": false,
	"globalNamespaces": "other,namespace",
	"daemonSocketDir": "/somewhere/socket",
	"socketDir": "/host/run/multus/socket"
}`))
	})
})
