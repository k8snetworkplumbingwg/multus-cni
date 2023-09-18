// Copyright (c) 2023 Multus Authors
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

// This binary submit CSR for kube controll access for multus thin plugin
// and generate Kubeconfig
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/template"

	"github.com/spf13/pflag"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var kubeConfigTemplate = `apiVersion: v1
clusters:
  - cluster:
      certificate-authority-data: {{.CADATA}}
      server: {{.K8S_APISERVER}}
    name: default-cluster
contexts:
  - context:
      cluster: default-cluster
      namespace: default
      user: default-auth
    name: default-context
current-context: default-context
kind: Config
preferences: {}
users:
  - name: default-auth
    user:
      client-certificate: {{.CERTDIR}}/multus-client-current.pem
      client-key: {{.CERTDIR}}/multus-client-current.pem
`

func main() {
	certDir := pflag.StringP("certdir", "", "/tmp", "specify cert directory")
	bootstrapConfig := pflag.StringP("bootstrap-config", "", "/tmp/kubeconfig", "specify bootstrap kubernetes config")
	kubeconfigPath := pflag.StringP("kubeconfig", "", "/run/multus/kubeconfig", "specify output kubeconfig path")
	helpFlag := pflag.BoolP("help", "h", false, "show help message and quit")

	pflag.Parse()
	if *helpFlag {
		pflag.PrintDefaults()
		os.Exit(1)
	}

	// check variables
	if _, err := os.Stat(*bootstrapConfig); err != nil {
		klog.Fatalf("failed to read bootstrap config %q", *bootstrapConfig)
	}
	st, err := os.Stat(*certDir)
	if err != nil {
		klog.Fatalf("failed to find cert directory %q", *certDir)
	}
	if !st.IsDir() {
		klog.Fatalf("cert directory %q is not directory", *certDir)
	}

	nodeName := os.Getenv("K8S_NODE")
	if nodeName == "" {
		klog.Fatalf("cannot identify node name from K8S_NODE env variables")
	}

	// retrieve API server from bootstrapConfig()
	config, err := clientcmd.BuildConfigFromFlags("", *bootstrapConfig)
	if err != nil {
		klog.Fatalf("cannot get in-cluster config: %v", err)
	}
	apiServer := fmt.Sprintf("%s%s", config.Host, config.APIPath)
	caData := base64.StdEncoding.EncodeToString(config.CAData)

	// run certManager to create certification
	if _, err = k8sclient.PerNodeK8sClient(nodeName, *bootstrapConfig, *certDir); err != nil {
		klog.Fatalf("failed to start cert manager: %v", err)
	}

	fp, err := os.OpenFile(*kubeconfigPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		klog.Fatalf("cannot create kubeconfig file %q: %v", *kubeconfigPath, err)
	}

	// render kubeconfig
	templateKubeconfig, err := template.New("kubeconfig").Parse(kubeConfigTemplate)
	if err != nil {
		klog.Fatalf("template parse error: %v", err)
	}
	templateData := map[string]string{
		"CADATA":        caData,
		"CERTDIR":       *certDir,
		"K8S_APISERVER": apiServer,
	}
	// genearate kubeconfig from template
	if err = templateKubeconfig.Execute(fp, templateData); err != nil {
		klog.Fatalf("cannot create kubeconfig: %v", err)
	}
	if err = fp.Close(); err != nil {
		klog.Fatalf("cannot save kubeconfig: %v", err)
	}

	klog.Infof("kubeconfig %q is saved", *kubeconfigPath)

	// wait for signal
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	<-sigterm
	klog.Infof("signal received. remove kubeconfig %q and quit.", *kubeconfigPath)
	err = os.Remove(*kubeconfigPath)
	if err != nil {
		klog.Errorf("failed to remove kubeconfig %q: %v", *kubeconfigPath, err)
	}
}
