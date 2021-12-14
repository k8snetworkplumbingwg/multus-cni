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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	)

	var configManager *Manager
	var multusConfigDir string
	var defaultCniConfig string

	BeforeEach(func() {
		var err error
		multusConfigDir, err = ioutil.TempDir("", "multus-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(multusConfigDir, 0755)).To(Succeed())
	})

	BeforeEach(func() {
		defaultCniConfig = fmt.Sprintf("%s/%s", multusConfigDir, primaryCNIPluginName)
		Expect(ioutil.WriteFile(defaultCniConfig, []byte(primaryCNIPluginTemplate), userRWPermission)).To(Succeed())

		multusConf, _ := NewMultusConfig(
			primaryCNIName,
			cniVersion,
			kubeconfig)
		var err error
		configManager, err = NewManagerWithExplicitPrimaryCNIPlugin(*multusConf, multusConfigDir, primaryCNIPluginName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(multusConfigDir)).To(Succeed())
	})

	It("Generates a configuration, based on the contents of the delegated CNI config file", func() {
		expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"0.4.0\",\"dns\":{},\"ipam\":{},\"name\":\"mycni-name\",\"type\":\"mycni\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
		config, err := configManager.GenerateConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(expectedResult))
	})

	Context("Updates to the delegate CNI configuration", func() {
		var (
			doneChannel chan struct{}
			stopChannel chan struct{}
		)

		BeforeEach(func() {
			doneChannel = make(chan struct{})
			stopChannel = make(chan struct{})
			go func() {
				Expect(configManager.MonitorDelegatedPluginConfiguration(stopChannel, doneChannel)).To(Succeed())
			}()
		})

		AfterEach(func() {
			go func() { stopChannel <- struct{}{} }()
			Eventually(<-doneChannel).Should(Equal(struct{}{}))
			close(doneChannel)
			close(stopChannel)
		})

		It("Trigger the re-generation of the Multus CNI configuration", func() {
			newCNIConfig := "{\"cniVersion\":\"0.4.0\",\"dns\":{},\"ipam\":{},\"name\":\"yoyo-newnet\",\"type\":\"mycni\"}"
			Expect(ioutil.WriteFile(defaultCniConfig, []byte(newCNIConfig), userRWPermission)).To(Succeed())

			multusCniConfigFile := fmt.Sprintf("%s/%s", multusConfigDir, multusConfigFileName)
			Eventually(func() (string, error) {
				multusCniData, err := ioutil.ReadFile(multusCniConfigFile)
				return string(multusCniData), err
			}).Should(Equal(multusConfigFromDelegate(newCNIConfig)))
		})
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

func multusConfigFromDelegate(delegateConfig string) string {
	return fmt.Sprintf("{\"cniVersion\":\"0.4.0\",\"delegates\":[%s],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}", delegateConfig)
}
