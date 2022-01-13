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
	"testing"
)

const (
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

func newMultusConfigWithDelegates(pluginName string, cniVersion string, kubeconfig string, primaryCNIPluginConfig interface{}, configOptions ...Option) (*MultusConf, error) {
	multusConfig, err := NewMultusConfig(pluginName, cniVersion, kubeconfig, configOptions...)
	if err != nil {
		return multusConfig, err
	}
	return multusConfig, multusConfig.Mutate(withDelegates(primaryCNIPluginConfig.(map[string]interface{})))
}

func TestBasicMultusConfig(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig)
	assertError(t, err, nil)
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithNamespaceIsolation(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithNamespaceIsolation())
	assertError(t, err, nil)
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"namespaceIsolation\":true,\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithReadinessIndicator(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithReadinessFileIndicator("/a/b/u/it-lives"))
	assertError(t, err, nil)
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"readinessindicatorfile\":\"/a/b/u/it-lives\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithLoggingConfiguration(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithLogLevel("notice"),
		WithLogToStdErr(),
		WithLogFile("/u/y/w/log.1"))
	assertError(t, err, nil)
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"logFile\":\"/u/y/w/log.1\",\"logLevel\":\"notice\",\"logToStderr\":true,\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithGlobalNamespace(t *testing.T) {
	const globalNamespace = "come-along-ns"
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithGlobalNamespaces(globalNamespace))
	assertError(t, err, nil)
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"globalNamespaces\":\"come-along-ns\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithAdditionalBinDir(t *testing.T) {
	const anotherCNIBinDir = "a-dir-somewhere"
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithAdditionalBinaryFileDir(anotherCNIBinDir))
	assertError(t, err, nil)
	expectedResult := "{\"binDir\":\"a-dir-somewhere\",\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilities(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		withCapabilities(
			documentHelper(`{"capabilities": {"portMappings": true}}`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilities(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		withCapabilities(
			documentHelper(`{"capabilities": {"portMappings": true, "tuning": true}}`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilitiesFilterOnlyEnabled(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		withCapabilities(
			documentHelper(`{"capabilities": {"portMappings": true, "tuning": false}}`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithMultipleCapabilitiesDefinedOnAPlugin(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		withCapabilities(
			documentHelper(`{"plugins": [ {"capabilities": {"portMappings": true, "tuning": true}} ] }`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilitiesDefinedOnMultiplePlugins(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		withCapabilities(
			documentHelper(`{"plugins": [ {"capabilities": { "portMappings": true }}, {"capabilities": { "tuning": true }} ]}`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true,\"tuning\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func TestMultusConfigWithCapabilitiesDefinedOnMultiplePluginsFilterOnlyEnabled(t *testing.T) {
	multusConfig, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
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
}`)))
	assertError(t, err, nil)
	expectedResult := "{\"capabilities\":{\"portMappings\":true},\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"multus-cni-network\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
}

func assertError(t *testing.T, actual error, expected error) {
	if actual != nil && expected != nil {
		if actual.Error() != expected.Error() {
			t.Fatalf("multus config generation failed.\nExpected:\n%v\nbut GOT:\n%v", expected.Error(), actual.Error())
		}
	}

	if actual == nil && expected != nil {
		t.Fatalf("multus config generation failed.\nExpected:\n%v\nbut didn't get error", expected.Error())
	} else if actual != nil && expected == nil {
		t.Fatalf("multus config generation failed.\nDidn't expect error\nbut GOT: %v\n", actual.Error())
	}
}

func invalidDelegateCNIVersion(delegateCNIVersion, multusCNIVersion string) error {
	return fmt.Errorf("delegate cni version is %s while top level cni version is %s", delegateCNIVersion, multusCNIVersion)
}

func TestVersionIncompatibility(t *testing.T) {
	const delegateCNIVersion = "0.3.0"

	primaryCNIConfigOld := primaryCNIConfig
	tmpVer := primaryCNIConfig["cniVersion"]
	primaryCNIConfig["cniVersion"] = delegateCNIVersion
	_, err := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfigOld)
	primaryCNIConfig["cniVersion"] = tmpVer

	assertError(t, invalidDelegateCNIVersion(delegateCNIVersion, cniVersion), err)
}

func TestMultusConfigWithOverriddenName(t *testing.T) {
	newNetworkName := "mega-net-2000"
	multusConfig, _ := newMultusConfigWithDelegates(
		primaryCNIName,
		cniVersion,
		kubeconfig,
		primaryCNIConfig,
		WithOverriddenName(newNetworkName))
	expectedResult := "{\"cniVersion\":\"0.4.0\",\"delegates\":[{\"cniVersion\":\"1.0.0\",\"dns\":\"{}\",\"ipam\":\"{}\",\"logFile\":\"/var/log/ovn-kubernetes/ovn-k8s-cni-overlay.log\",\"logLevel\":\"5\",\"logfile-maxage\":5,\"logfile-maxbackups\":5,\"logfile-maxsize\":100,\"name\":\"ovn-kubernetes\",\"type\":\"ovn-k8s-cni-overlay\"}],\"kubeconfig\":\"/a/b/c/kubeconfig.kubeconfig\",\"name\":\"mega-net-2000\",\"type\":\"myCNI\"}"
	newTestCase(t, multusConfig.Generate).assertResult(expectedResult)
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
