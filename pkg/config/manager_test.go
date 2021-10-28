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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/fsnotify/fsnotify"
)

const suiteName = "Configuration Manager"

func TestMultusConfigurationManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, suiteName)
}

var _ = Describe(suiteName, func() {
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
		tmpDir = "/tmp"
	)

	var configManager *Manager
	var multusConfigFilePath string

	BeforeEach(func() {
		format.TruncatedDiff = false
		multusConfigFilePath = fmt.Sprintf("%s/%s", tmpDir, primaryCNIPluginName)
		Expect(ioutil.WriteFile(multusConfigFilePath, []byte(primaryCNIPluginTemplate), userRWPermission)).To(Succeed())

		multusConf := NewMultusConfig(
			primaryCNIName,
			cniVersion,
			kubeconfig)
		var err error
		configManager, err = NewManagerWithExplicitPrimaryCNIPlugin(*multusConf, tmpDir, primaryCNIPluginName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.Remove(multusConfigFilePath)).To(Succeed())
	})

	It("Generates a configuration, based on the contents of the delegated CNI config file", func() {
		expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"0.4.0\",\"dns\":{},\"ipam\":{},\"name\":\"mycni-name\",\"type\":\"mycni\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
		config, err := configManager.GenerateConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(expectedResult))
	})

	It("Reacts to updates of the delegated CNI configuration", func() {
		stopChan := make(chan struct{})
		doneChan := make(chan struct{})
		go func(stopChannel, doneChan chan struct{}) {
			Expect(configManager.MonitorDelegatedPluginConfiguration(stopChannel, doneChan)).To(Succeed())
		}(stopChan, doneChan)

		newCNIConfig := "{\"cniVersion\":\"0.4.0\",\"dns\":{},\"ipam\":{},\"name\":\"mycni-name\",\"type\":\"mycni\"}"
		Expect(ioutil.WriteFile(multusConfigFilePath, []byte(newCNIConfig), userRWPermission)).To(Succeed())
		Eventually(<-configManager.configWatcher.Events, time.Second).Should(Equal(
			fsnotify.Event{
				Name: multusConfigFilePath,
				Op:   fsnotify.Write,
			}))

		bytes, err := json.Marshal(configManager.cniConfigData)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(bytes)).To(Equal(newCNIConfig))

		// cleanup the watcher
		go func() { stopChan <- struct{}{} }()
		Eventually(<-doneChan, time.Second).Should(Equal(struct{}{}))
	})

	When("the user requests the name of the multus configuration to be overridden", func() {
		BeforeEach(func() {
			Expect(configManager.OverrideNetworkName()).To(Succeed())
		})

		It("Overrides the name of the multus configuration when requested", func() {
			expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"0.4.0\",\"dns\":{},\"ipam\":{},\"name\":\"mycni-name\",\"type\":\"mycni\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"mycni-name\",\"type\":\"myCNI\"}"
			config, err := configManager.GenerateConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(config).To(Equal(expectedResult))
		})
	})
})
