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

package k8sclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/transport"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog"

	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
)

const (
	certNamePrefix       = "multus-client"
	certCommonNamePrefix = "system:multus"
	certOrganization     = "system:multus"
)

var (
	certUsages = []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth}
)

// getPerNodeKubeconfig creates new kubeConfig, based on bootstrap, with new certDir
func getPerNodeKubeconfig(bootstrap *rest.Config, certDir string) *rest.Config {
	return &rest.Config{
		Host:    bootstrap.Host,
		APIPath: bootstrap.APIPath,
		ContentConfig: rest.ContentConfig{
			AcceptContentTypes: "application/vnd.kubernetes.protobuf,application/json",
			ContentType:        "application/vnd.kubernetes.protobuf",
		},
		TLSClientConfig: rest.TLSClientConfig{
			KeyFile:  path.Join(certDir, certNamePrefix+"-current.pem"),
			CertFile: path.Join(certDir, certNamePrefix+"-current.pem"),
			CAData:   bootstrap.TLSClientConfig.CAData,
		},
		// Allow multus (especially in server mode) to make more concurrent requests
		// to reduce client-side throttling
		QPS:   50,
		Burst: 50,
		// Set the config timeout to one minute.
		Timeout: time.Minute,
	}
}

// PerNodeK8sClient creates/reload new multus kubeconfig per-node.
func PerNodeK8sClient(nodeName, bootstrapKubeconfigFile string, certDuration time.Duration, certDir string) (*ClientInfo, error) {
	bootstrapKubeconfig, err := clientcmd.BuildConfigFromFlags("", bootstrapKubeconfigFile)
	if err != nil {
		return nil, logging.Errorf("failed to load bootstrap kubeconfig %s: %v", bootstrapKubeconfigFile, err)
	}
	config := getPerNodeKubeconfig(bootstrapKubeconfig, certDir)

	// If we have a valid certificate, user that to fetch CSRs.
	// Otherwise, use the bootstrap credentials from bootstrapKubeconfig
	// https://github.com/kubernetes/kubernetes/blob/068ee321bc7bfe1c2cefb87fb4d9e5deea84fbc8/cmd/kubelet/app/server.go#L953-L963
	newClientsetFn := func(current *tls.Certificate) (kubernetes.Interface, error) {
		cfg := bootstrapKubeconfig

		// validate the kubeconfig
		tempClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			logging.Errorf("failed to read kubeconfig from cert manager: %v", err)
		} else {
			_, err := tempClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
			// tls unknown authority error is unrecoverable error with retry
			if err != nil {
				if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
					logging.Verbosef("cert mgr gets invalid config. rebuild from bootstrap kubeconfig")
					// reload and use bootstrapKubeconfig again
					newBootstrapKubeconfig, _ := clientcmd.BuildConfigFromFlags("", bootstrapKubeconfigFile)
					cfg = newBootstrapKubeconfig
				} else {
					logging.Errorf("failed to list pods with new certs: %v", err)
				}
			}

			if current != nil {
				cfg = config
			}
		}
		return kubernetes.NewForConfig(cfg)
	}

	certificateStore, err := certificate.NewFileStore(certNamePrefix, certDir, certDir, "", "")
	if err != nil {
		return nil, logging.Errorf("failed to initialize the certificate store: %v", err)
	}

	certManager, err := certificate.NewManager(&certificate.Config{
		ClientsetFn: newClientsetFn,
		Template: &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   fmt.Sprintf("%s:%s", certCommonNamePrefix, nodeName),
				Organization: []string{certOrganization},
			},
		},
		RequestedCertificateLifetime: &certDuration,
		SignerName:                   certificatesv1.KubeAPIServerClientSignerName,
		Usages:                       certUsages,
		CertificateStore:             certificateStore,
	})
	if err != nil {
		return nil, logging.Errorf("failed to initialize the certificate manager: %v", err)
	}
	if certDuration < time.Hour {
		// the default value for CertCallbackRefreshDuration (5min) is too long for short-lived certs,
		// set it to a more sensible value
		transport.CertCallbackRefreshDuration = time.Second * 10
	}
	certManager.Start()

	logging.Verbosef("Waiting for certificate")
	var storeErr error
	err = wait.PollWithContext(context.TODO(), time.Second, 2*time.Minute, func(_ context.Context) (bool, error) {
		var currentCert *tls.Certificate
		currentCert, storeErr = certificateStore.Current()
		return currentCert != nil && storeErr == nil, nil
	})
	if err != nil {
		return nil, logging.Errorf("certificate was not signed, last cert store err: %v err: %v", storeErr, err)
	}
	logging.Verbosef("Certificate found!")

	return newClientInfo(config)
}

// InClusterK8sClient returns the `k8s.ClientInfo` struct to use to connect to
// the k8s API.
func InClusterK8sClient() (*ClientInfo, error) {
	clientInfo, err := GetK8sClient("", nil)
	if err != nil {
		return nil, err
	}
	if clientInfo == nil {
		return nil, fmt.Errorf("failed to create in-cluster kube client")
	}
	return clientInfo, err
}

// SetK8sClientInformers adds informer structure to ClientInfo to utilize in thick daemon
func (c *ClientInfo) SetK8sClientInformers(podInformer, netDefInformer cache.SharedIndexInformer) {
	c.PodInformer = podInformer
	c.NetDefInformer = netDefInformer
}

// GetK8sClient gets client info from kubeconfig
func GetK8sClient(kubeconfig string, kubeClient *ClientInfo) (*ClientInfo, error) {
	logging.Debugf("GetK8sClient: %s, %v", kubeconfig, kubeClient)
	// If we get a valid kubeClient (eg from testcases) just return that
	// one.
	if kubeClient != nil {
		return kubeClient, nil
	}

	var err error
	var config *rest.Config

	// Otherwise try to create a kubeClient from a given kubeConfig
	if kubeconfig != "" {
		// uses the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, logging.Errorf("GetK8sClient: failed to get context for the kubeconfig %v: %v", kubeconfig, err)
		}
	} else if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// Try in-cluster config where multus might be running in a kubernetes pod
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, logging.Errorf("GetK8sClient: failed to get context for in-cluster kube config: %v", err)
		}
	} else {
		// No kubernetes config; assume we shouldn't talk to Kube at all
		return nil, nil
	}

	// Specify that we use gRPC
	config.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	config.ContentType = "application/vnd.kubernetes.protobuf"
	// Set the config timeout to one minute.
	config.Timeout = time.Minute
	// Allow multus (especially in server mode) to make more concurrent requests
	// to reduce client-side throttling
	config.QPS = 50
	config.Burst = 50

	return newClientInfo(config)
}

// newClientInfo returns a `ClientInfo` from a configuration created from an
// existing kubeconfig file.
func newClientInfo(config *rest.Config) (*ClientInfo, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	netclient, err := netclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: client.CoreV1().Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "multus"})
	return &ClientInfo{
		Client:           client,
		NetClient:        netclient,
		EventBroadcaster: broadcaster,
		EventRecorder:    recorder,
	}, nil
}
