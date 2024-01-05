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

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	primaryCNIName = "myCNI"
	cniVersion     = "0.4.0"
)

type testCase struct {
	t                        *testing.T
	configGenerationFunction func() (string, error)
}

var primaryCNIConfig = map[string]interface{}{
	"cniVersion":         "1.0.0",
	"name":               "ovn-kubernetes",
	"type":               "ovn-k8s-cni-overlay",
	"ipam":               "{}",
	"dns":                "{}",
	"logFile":            "/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log",
	"logLevel":           "5",
	"logfile-maxsize":    100,
	"logfile-maxbackups": 5,
	"logfile-maxage":     5,
}
var primaryCNIFile = "/etc/cni/net.d/10-flannel.conf"

var _ = Describe("Configuration Generator", func() {
	var tmpDir string
	var err error

	BeforeEach(func() {
		tmpDir, err = os.MkdirTemp("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("basic multus config", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with capabilities", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(documentHelper(`{"capabilities": {"portMappings": true}}`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{
					"portMappings":true
				},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(
			documentHelper(`{"capabilities": {"portMappings": true, "tuning": true}}`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities filter only enabled", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(
			documentHelper(`{"capabilities": {"portMappings": true, "tuning": false}}`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on a plugin", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(
			documentHelper(
				`{"plugins": [ {"capabilities": {"portMappings": true, "tuning": true}} ] }`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on multiple plugins", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(
			documentHelper(`
				{
					"plugins": [
					{
						"capabilities": { "portMappings": true }
					}, 
					{
						"capabilities": { "tuning": true }
					}
					]
				}`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on multiple plugins filter only enabled", func() {
		multusConfFile := fmt.Sprintf(`{
			"name": %q,
			"cniVersion": %q,
			"clusterNetwork": %q
		}`, primaryCNIName, cniVersion, primaryCNIFile)
		multusConfFileName := fmt.Sprintf("%s/10-testcni.conf", tmpDir)
		Expect(os.WriteFile(multusConfFileName, []byte(multusConfFile), 0755)).To(Succeed())

		multusConfig, err := ParseMultusConfig(multusConfFileName)
		Expect(err).NotTo(HaveOccurred())
		Expect(multusConfig.setCapabilities(
			documentHelper(`
				{
					"plugins": [
					{
						"capabilities": {
							"portMappings": true
						}
					},
					{
						"capabilities": {
							"tuning": false
						}
					}
					]
				}`))).To(Succeed())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"multus-shim"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})
})

func documentHelper(pluginInfo string) interface{} {
	dp, _ := documentCNIData([]byte(pluginInfo))
	return dp
}

func documentCNIData(masterCNIConfigData []byte) (interface{}, error) {
	var cniData interface{}
	if err := json.Unmarshal(masterCNIConfigData, &cniData); err != nil {
		return nil, fmt.Errorf("failed to unmarshall the delegate CNI configuration: %w", err)
	}
	return cniData, nil
}
