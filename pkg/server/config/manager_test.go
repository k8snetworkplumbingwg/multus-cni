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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

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
		multusConfigDir, err = os.MkdirTemp("", "multus-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(multusConfigDir, 0755)).To(Succeed())

		defaultCniConfig = fmt.Sprintf("%s/%s", multusConfigDir, primaryCNIPluginName)
		Expect(os.WriteFile(defaultCniConfig, []byte(primaryCNIPluginTemplate), UserRWPermission)).To(Succeed())

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

	It("Check overrideCNIVersion is worked", func() {
		err := overrideCNIVersion(defaultCniConfig, "1.1.1")
		Expect(err).NotTo(HaveOccurred())
		raw, err := os.ReadFile(defaultCniConfig)
		Expect(err).NotTo(HaveOccurred())

		var jsonConfig map[string]interface{}
		err = json.Unmarshal(raw, &jsonConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonConfig["cniVersion"].(string)).To(Equal("1.1.1"))
	})

	It("Check primaryCNIPlugin can be identified", func() {
		fileName, err := getPrimaryCNIPluginName(multusConfigDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileName).To(Equal(primaryCNIPluginName))
	})

	It("Check MonitorPluginConfiguration", func() {
		config, err := configManager.GenerateConfig()
		Expect(err).NotTo(HaveOccurred())

		err = configManager.PersistMultusConfig(config)
		Expect(err).NotTo(HaveOccurred())

		configWatcherDoneChannel := make(chan struct{})
		go func(stopChannel chan struct{}, doneChannel chan struct{}) {
			err := configManager.MonitorPluginConfiguration(configWatcherDoneChannel, stopChannel)
			Expect(err).NotTo(HaveOccurred())
		}(make(chan struct{}), configWatcherDoneChannel)

		updatedCNIConfig := `
{
  "cniVersion": "0.4.0",
  "name": "mycni-name",
  "type": "mycni2",
  "ipam": {},
  "dns": {}
}
`
		// update the CNI config to update the master config
		Expect(os.WriteFile(defaultCniConfig, []byte(updatedCNIConfig), UserRWPermission)).To(Succeed())

		// wait for a while to get fsnotify event
		time.Sleep(100 * time.Millisecond)
		file, err := os.ReadFile(configManager.multusConfigFilePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(file)).To(Equal(config))

		// stop groutine
		configWatcherDoneChannel <- struct{}{}
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
		multusConfigDir, err = os.MkdirTemp("", "multus-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(multusConfigDir, 0755)).To(Succeed())

		defaultCniConfig = fmt.Sprintf("%s/%s", multusConfigDir, primaryCNIPluginName)
		Expect(os.WriteFile(defaultCniConfig, []byte(primaryCNIPluginTemplate), UserRWPermission)).To(Succeed())

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
