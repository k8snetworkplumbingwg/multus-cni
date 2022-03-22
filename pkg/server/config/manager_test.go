// Copyright (c) 2021 Multus Authors
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

package config

import (
	"fmt"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configuration Manager", func() {
	const (
		primaryCNIPluginName     = "00-mycni.conf"
		primaryCNIPluginTemplate = `
{
  "cniVersion": "0.4.0",
  "name": "mycni-name",
  "type": "mycni",
  "ipam": {},
  "dns": {}
}
`
	)

	var configManager *Manager
	var multusConfigDir string
	var defaultCniConfig string

	BeforeEach(func() {
		var err error
		multusConfigDir, err = ioutil.TempDir("", "multus-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(multusConfigDir, 0755)).To(Succeed())

		defaultCniConfig = fmt.Sprintf("%s/%s", multusConfigDir, primaryCNIPluginName)
		Expect(ioutil.WriteFile(defaultCniConfig, []byte(primaryCNIPluginTemplate), UserRWPermission)).To(Succeed())

		multusConf, _ := NewMultusConfig(
			primaryCNIName,
			cniVersion)
		configManager, err = NewManagerWithExplicitPrimaryCNIPlugin(*multusConf, multusConfigDir, primaryCNIPluginName, false)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(multusConfigDir)).To(Succeed())
	})

	It("Generates a configuration, based on the contents of the delegated CNI config file", func() {
		expectedResult := fmt.Sprintf("{\"cniVersion\":\"0.4.0\",\"name\":\"multus-cni-network\",\"clusterNetwork\":\"%s\",\"type\":\"myCNI\"}", defaultCniConfig)
		config, err := configManager.GenerateConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(expectedResult))
	})

	When("the user requests the name of the multus configuration to be overridden", func() {
		BeforeEach(func() {
			Expect(configManager.OverrideNetworkName()).To(Succeed())
		})

		It("Overrides the name of the multus configuration when requested", func() {
			expectedResult := fmt.Sprintf("{\"cniVersion\":\"0.4.0\",\"name\":\"mycni-name\",\"clusterNetwork\":\"%s\",\"type\":\"myCNI\"}", defaultCniConfig)
			config, err := configManager.GenerateConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(config).To(Equal(expectedResult))
		})
	})
})

var _ = Describe("Configuration Manager with mismatched cniVersion", func() {
	const (
		primaryCNIPluginName     = "00-mycni.conf"
		primaryCNIPluginTemplate = `
{
  "cniVersion": "0.3.1",
  "name": "mycni-name",
  "type": "mycni",
  "ipam": {},
  "dns": {}
}
`
	)

	var multusConfigDir string
	var defaultCniConfig string

	It("test cni version incompatibility", func() {
		var err error
		multusConfigDir, err = ioutil.TempDir("", "multus-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(multusConfigDir, 0755)).To(Succeed())

		defaultCniConfig = fmt.Sprintf("%s/%s", multusConfigDir, primaryCNIPluginName)
		Expect(ioutil.WriteFile(defaultCniConfig, []byte(primaryCNIPluginTemplate), UserRWPermission)).To(Succeed())

		multusConf, _ := NewMultusConfig(
			primaryCNIName,
			cniVersion)
		_, err = NewManagerWithExplicitPrimaryCNIPlugin(*multusConf, multusConfigDir, primaryCNIPluginName, false)
		Expect(err).To(MatchError("failed to load the primary CNI configuration as a multus delegate with error 'delegate cni version is 0.3.1 while top level cni version is 0.4.0'"))
	})

	AfterEach(func() {
		Expect(os.RemoveAll(multusConfigDir)).To(Succeed())
	})

})
