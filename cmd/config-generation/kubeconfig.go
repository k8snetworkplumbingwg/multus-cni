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
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

const userRWPermission = 0600

const (
	cniConfigDirVarName   = "cni-config-dir"
	k8sCAFilePathVarName  = "kube-ca-file"
	k8sServiceHostVarName = "k8s-service-host"
	k8sServicePortVarName = "k8s-service-port"
	serviceAccountPath    = "/var/run/secrets/kubernetes.io/serviceaccount"
	skipTLSVerifyVarName  = "skip-tls-verify"
)

const (
	defaultCniConfigDir   = "/host/etc/cni/net.d"
	defaultK8sCAFilePath  = ""
	defaultK8sServiceHost = ""
	defaultK8sServicePort = 0
	defaultSkipTLSValue   = false
)

func main() {
	k8sServiceHost := flag.String(k8sServiceHostVarName, defaultK8sServiceHost, "Cluster IP of the kubernetes service")
	k8sServicePort := flag.Int(k8sServicePortVarName, defaultK8sServicePort, "Port of the kubernetes service")
	skipTLSVerify := flag.Bool(skipTLSVerifyVarName, defaultSkipTLSValue, "Should TLS verification be skipped")
	kubeCAFilePath := flag.String(k8sCAFilePathVarName, defaultK8sCAFilePath, "Override the default kubernetes CA file path")
	cniConfigDir := flag.String(cniConfigDirVarName, defaultCniConfigDir, "CNI config dir")
	flag.Parse()

	if *k8sServiceHost == defaultK8sServiceHost {
		logInvalidArg("must provide the k8s service cluster port")
	}
	if *k8sServicePort == defaultK8sServicePort {
		logInvalidArg("must provide the k8s service cluster port")
	}
	if *kubeCAFilePath == defaultK8sServiceHost {
		*kubeCAFilePath = serviceAccountPath + "/ca.crt"
	}

	tlsCfg := "insecure-skip-tls-verify: true"
	if !*skipTLSVerify {
		kubeCAFileContents, err := k8sCAFileContentsBase64(*kubeCAFilePath)
		if err != nil {
			logError("failed grabbing CA file: %w", err)
		}
		tlsCfg = "certificate-authority-data: " + kubeCAFileContents
	}

	multusConfigDir := *cniConfigDir + "/multus.d/"
	if err := prepareCNIConfigDir(multusConfigDir); err != nil {
		logError("failed to create CNI config dir: %w", err)
	}
	kubeConfigFilePath := *cniConfigDir + "/multus.d/multus.kubeconfig"
	serviceAccountToken, err := k8sKubeConfigToken(serviceAccountPath + "/token")
	if err != nil {
		logError("failed grabbing k8s token: %w", err)
	}
	if err := writeKubeConfig(kubeConfigFilePath, "https", *k8sServiceHost, *k8sServicePort, tlsCfg, serviceAccountToken); err != nil {
		logError("failed generating kubeconfig: %w", err)
	}
}

func k8sCAFileContentsBase64(pathCAFile string) (string, error) {
	data, err := ioutil.ReadFile(pathCAFile)
	if err != nil {
		return "", fmt.Errorf("failed reading file %s: %w", pathCAFile, err)
	}
	return strings.Trim(base64.StdEncoding.EncodeToString(data), "\n"), nil
}

func k8sKubeConfigToken(tokenPath string) (string, error) {
	data, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed reading file %s: %w", tokenPath, err)
	}
	return string(data), nil
}

func writeKubeConfig(outputPath string, protocol string, k8sServiceIP string, k8sServicePort int, tlsConfig string, serviceAccountToken string) error {
	kubeConfigTemplate := `
# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: %s://[%s]:%d
    %s
users:
- name: multus
  user:
    token: "%s"
contexts:
- name: multus-context
  context:
    cluster: local
    user: multus
current-context: multus-context
`
	kubeconfig := fmt.Sprintf(kubeConfigTemplate, protocol, k8sServiceIP, k8sServicePort, tlsConfig, serviceAccountToken)
	logInfo("Generated KubeConfig: \n%s", kubeconfig)
	return ioutil.WriteFile(outputPath, []byte(kubeconfig), userRWPermission)
}

func prepareCNIConfigDir(cniConfigDirPath string) error {
	return os.MkdirAll(cniConfigDirPath, userRWPermission)
}

func logInvalidArg(format string, values ...interface{}) {
	log.Printf("ERROR: %s", fmt.Errorf(format, values...).Error())
	flag.PrintDefaults()
	os.Exit(1)
}

func logError(format string, values ...interface{}) {
	log.Printf("ERROR: %s", fmt.Errorf(format, values...).Error())
	os.Exit(1)
}

func logInfo(format string, values ...interface{}) {
	log.Printf("INFO: %s", fmt.Sprintf(format, values...))
}
