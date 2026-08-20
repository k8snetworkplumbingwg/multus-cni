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
	"time"

	"github.com/spf13/pflag"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/cmdutils"
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
	kubeconfigPathRaw := pflag.StringP("kubeconfig", "", "/run/multus/kubeconfig", "specify output kubeconfig path")
	certDurationString := pflag.StringP("cert-duration", "", "10m", "specify certificate duration")
	helpFlag := pflag.BoolP("help", "h", false, "show help message and quit")

	pflag.Parse()
	if *helpFlag {
		pflag.PrintDefaults()
		os.Exit(1)
	}

	kubeconfigPath, err := cmdutils.NewRootedFile(*kubeconfigPathRaw)
	if err != nil {
		klog.Fatalf("illegal path in kubeconfigPath %s: %v", *kubeconfigPathRaw, err)
	}
	defer kubeconfigPath.Close()

	bootstrapConfigPath, err := cmdutils.NewRootedFile(*bootstrapConfig)
	if err != nil {
		klog.Fatalf("illegal path in bootstrap-config %s: %v", *bootstrapConfig, err)
	}
	defer bootstrapConfigPath.Close()

	certDirPath, err := cmdutils.NewRootedDir(*certDir)
	if err != nil {
		klog.Fatalf("illegal path in certdir %s: %v", *certDir, err)
	}
	defer certDirPath.Close()

	// check variables
	if _, err := bootstrapConfigPath.Root.Stat(bootstrapConfigPath.FileName); err != nil {
		klog.Fatalf("failed to read bootstrap config %q", bootstrapConfigPath.Path())
	}
	st, err := certDirPath.Root.Stat(".")
	if err != nil {
		klog.Fatalf("failed to find cert directory %q", certDirPath.Path())
	}
	if !st.IsDir() {
		klog.Fatalf("cert directory %q is not directory", certDirPath.Path())
	}
	certDuration, err := time.ParseDuration(*certDurationString)
	if err != nil {
		klog.Fatalf("failed to parse duration %q: %v", *certDurationString, err)
	}

	nodeName := os.Getenv("MULTUS_NODE_NAME")
	if nodeName == "" {
		klog.Fatalf("cannot identify node name from MULTUS_NODE_NAME env variables")
	}

	// retrieve API server from bootstrapConfig()
	config, err := clientcmd.BuildConfigFromFlags("", bootstrapConfigPath.Path())
	if err != nil {
		klog.Fatalf("cannot get in-cluster config: %v", err)
	}
	apiServer := fmt.Sprintf("%s%s", config.Host, config.APIPath)
	caData := base64.StdEncoding.EncodeToString(config.CAData)

	// run certManager to create certification
	if _, err = k8sclient.PerNodeK8sClient(nodeName, bootstrapConfigPath.Path(), certDuration, certDirPath.Path()); err != nil {
		klog.Fatalf("failed to start cert manager: %v", err)
	}

	templateData := map[string]string{
		"CADATA":        caData,
		"CERTDIR":       certDirPath.Path(),
		"K8S_APISERVER": apiServer,
	}
	if err = writeKubeconfig(kubeconfigPath, templateData); err != nil {
		klog.Fatalf("%v", err)
	}

	klog.Infof("kubeconfig %q is saved", kubeconfigPath.Path())

	// wait for signal
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)
	<-sigterm
	klog.Infof("signal received. remove kubeconfig %q and quit.", kubeconfigPath.Path())
	err = kubeconfigPath.Root.Remove(kubeconfigPath.FileName)
	if err != nil {
		klog.Errorf("failed to remove kubeconfig %q: %v", kubeconfigPath.Path(), err)
	}
}

func writeKubeconfig(kubeconfigPath *cmdutils.RootedFile, templateData map[string]string) (err error) {
	fp, err := kubeconfigPath.Root.OpenFile(kubeconfigPath.FileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("cannot create kubeconfig file %q: %w", kubeconfigPath.Path(), err)
	}
	defer func() {
		if closeErr := fp.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("cannot save kubeconfig file %q: %w", kubeconfigPath.Path(), closeErr)
		}
	}()
	if err = fp.Chmod(0600); err != nil {
		return fmt.Errorf("cannot set kubeconfig file mode %q: %w", kubeconfigPath.Path(), err)
	}

	templateKubeconfig, err := template.New("kubeconfig").Parse(kubeConfigTemplate)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}
	if err = templateKubeconfig.Execute(fp, templateData); err != nil {
		return fmt.Errorf("cannot create kubeconfig: %w", err)
	}

	return nil
}
