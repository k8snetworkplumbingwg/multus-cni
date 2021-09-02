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

package main

import (
	"testing"
)

const (
	networkName    = "multus-cni-network"
	primaryCNIName = "myCNI"
	cniVersion     = "0.4.0"
	kubeconfig     = "/a/b/c/kubeconfig.kubeconfig"
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

func TestBasicMultusConfig(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithNamespaceIsolation(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withNamespaceIsolation()
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"namespaceIsolation\":true}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithReadinessIndicator(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withReadinessFileIndicator("/a/b/u/it-lives")
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"readinessindicatorfile\":\"/a/b/u/it-lives\"}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithLoggingConfiguration(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withLogLevel("notice")
	multusConfig.withLogToStdErr()
	multusConfig.withLogFile("/u/y/w/log.1")
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"logFile\":\"/u/y/w/log.1\",\"logLevel\":\"notice\",\"logToStderr\":true}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithGlobalNamespace(t *testing.T) {
	const globalNamespace = "come-along-ns"
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withGlobalNamespaces(globalNamespace)
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"globalNamespaces\":\"come-along-ns\"}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithAdditionalBinDir(t *testing.T) {
	const anotherCNIBinDir = "a-dir-somewhere"
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withAdditionalBinaryFileDir(anotherCNIBinDir)
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"binDir\":\"a-dir-somewhere\",\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilities(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{Capabilities: map[string]bool{"portMappings": true}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilities(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{Capabilities: map[string]bool{"portMappings": true, "tuning": true}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilitiesFilterOnlyEnabled(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{Capabilities: map[string]bool{"portMappings": true, "tuning": false}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilitiesDefinedOnAPlugin(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{
			Plugins: []*delegatedPluginConf{{
				Capabilities: map[string]bool{"portMappings": true, "tuning": true},
			}}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilitiesDefinedOnMultiplePlugins(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{
			Plugins: []*delegatedPluginConf{
				{Capabilities: map[string]bool{"portMappings": true}},
				{Capabilities: map[string]bool{"tuning": true}},
			}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilitiesDefinedOnMultiplePluginsFilterOnlyEnabled(t *testing.T) {
	multusConfig := newMultusConfig(
		networkName,
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	multusConfig.withCapabilities(
		primaryCNIConfigData{
			Plugins: []*delegatedPluginConf{
				{Capabilities: map[string]bool{"portMappings": true}},
				{Capabilities: map[string]bool{"tuning": false}},
			}})
	expectedResult := "{\"name\":\"multus-cni-network\",\"type\":\"myCNI\",\"cniVersion\":\"0.4.0\",\"capabilities\":{\"portMappings\":true},\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}]}"
	newTestCase(t, multusConfig.generate).assertResult(expectedResult)
}

func newTestCase(t *testing.T, configGenerationFunc func() (string, error)) *testCase {
	return &testCase{
		t:                        t,
		configGenerationFunction: configGenerationFunc,
	}
}

func (tc testCase) assertResult(expectedResult string) {
	multusCNIConfig, err := tc.configGenerationFunction()
	if err != nil {
		tc.t.Fatalf("error generating multus configuration: %v", err)
	}
	if multusCNIConfig != expectedResult {
		tc.t.Fatalf("multus config generation failed.\nExpected:\n%s\nbut GOT:\n%s", expectedResult, multusCNIConfig)
	}
}
