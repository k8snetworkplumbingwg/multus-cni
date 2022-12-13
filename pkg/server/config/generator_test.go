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
	"testing"

	testutils "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/testing"

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

func newMultusConfigWithDelegates(pluginName string, cniVersion string, primaryCNIFile string, configOptions ...Option) (*MultusConf, error) {
	multusConfig, err := NewMultusConfig(pluginName, cniVersion, configOptions...)
	if err != nil {
		return multusConfig, err
	}
	return multusConfig, multusConfig.Mutate(withClusterNetwork(primaryCNIFile))
}

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
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with namespaceisolation", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithNamespaceIsolation())
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"namespaceIsolation":true,
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with readinessindicator", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithReadinessFileIndicator("/a/b/u/it-lives"))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"readinessindicatorfile":"/a/b/u/it-lives",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging configuration", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogLevel("notice"),
			WithLogToStdErr(),
			WithLogFile("/u/y/w/log.1"))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"logFile":"/u/y/w/log.1",
				"logLevel":"notice",
				"logToStderr":true, 
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging options configuration", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogOptions(&LogOptions{
				MaxAge:     testutils.Int(5),
				MaxSize:    testutils.Int(100),
				MaxBackups: testutils.Int(5),
				Compress:   testutils.Bool(true),
			}))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"logOptions": {
					"maxAge": 5,
					"maxSize": 100,
					"maxBackups": 5,
					"compress": true
				},
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging options with max age", func() {
		logOption := &LogOptions{}
		MutateLogOptions(logOption, WithLogMaxAge(testutils.Int(5)))
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogOptions(logOption))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"logOptions": {
					"maxAge": 5
				},
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging options with max size", func() {
		logOption := &LogOptions{}
		MutateLogOptions(logOption, WithLogMaxSize(testutils.Int(100)))
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogOptions(logOption))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"logOptions": {
					"maxSize": 100
				},
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging options with log max backups", func() {
		logOption := &LogOptions{}
		MutateLogOptions(logOption, WithLogMaxBackups(testutils.Int(5)))
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogOptions(logOption))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"logOptions": {
					"maxBackups": 5
				},
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with logging options with log compress", func() {
		logOption := &LogOptions{}
		MutateLogOptions(logOption, WithLogCompress(testutils.Bool(true)))
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithLogOptions(logOption))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"logOptions": {
					"compress": true
				},
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("generates a multus config with CNI configuration directory", func() {
		cniConfigDir := "/host/etc/cni/net.d"
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithCniConfigDir(cniConfigDir))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"cniConfigDir":"/host/etc/cni/net.d",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("generates a multus config with a custom socket directory", func() {
		socketDir := "/var/run/multus/multussock/"
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithSocketDir(socketDir))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"socketDir":"/var/run/multus/multussock/",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with global namespace", func() {
		const globalNamespace = "come-along-ns"
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithGlobalNamespaces(globalNamespace))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"globalNamespaces":"come-along-ns",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with additional binDir", func() {
		const anotherCNIBinDir = "a-dir-somewhere"
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithAdditionalBinaryFileDir(anotherCNIBinDir))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"binDir":"a-dir-somewhere",
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with capabilities", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
				documentHelper(`{"capabilities": {"portMappings": true}}`)),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{
					"portMappings":true
				},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
				documentHelper(`{"capabilities": {"portMappings": true, "tuning": true}}`)),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities filter only enabled", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
				documentHelper(`{"capabilities": {"portMappings": true, "tuning": false}}`)),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on a plugin", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
				documentHelper(`{"plugins": [ {"capabilities": {"portMappings": true, "tuning": true}} ] }`)),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on multiple plugins", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
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
				}`)),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true,"tuning":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with multiple capabilities defined on multiple plugins filter only enabled", func() {
		multusConfig, err := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			withCapabilities(
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
				}`),
			),
		)
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
			{
				"capabilities":{"portMappings":true},
				"cniVersion":"0.4.0",
				"clusterNetwork":"%s",
				"name":"multus-cni-network",
				"type":"myCNI"
			}`, primaryCNIFile)
		Expect(multusConfig.Generate()).Should(MatchJSON(expectedResult))
	})

	It("multus config with overridden name", func() {
		newNetworkName := "mega-net-2000"
		multusConfig, _ := newMultusConfigWithDelegates(
			primaryCNIName,
			cniVersion,
			primaryCNIFile,
			WithOverriddenName(newNetworkName))
		Expect(err).NotTo(HaveOccurred())
		expectedResult := fmt.Sprintf(`
		{
			"cniVersion":"0.4.0",
			"clusterNetwork":"%s",
			"name":"mega-net-2000",
			"type":"myCNI"
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
