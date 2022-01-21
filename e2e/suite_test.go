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

package e2e

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	k8splumbersclientset "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
)

type multusClient struct {
	k8sClientSet *kubernetes.Clientset
	nadClientSet *k8splumbersclientset.Clientset
}

var kubeconfig *string
var clientset *multusClient

func TestMultus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multus e2e suite")
}

var _ = BeforeSuite(func() {
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	Expect(err).NotTo(HaveOccurred(), "could not retrieve the kubeconfig to contact the k8s cluster")

	k8sClientSet, err := kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred(), "could not create the kubeclient from the retrieved kubeconfig")

	k8sPlumbersClientSet, err := k8splumbersclientset.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred(), "could not create the kubeclient from the retrieved kubeconfig")
	clientset = &multusClient{
		k8sClientSet: k8sClientSet,
		nadClientSet: k8sPlumbersClientSet,
	}
})

func init() {
	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
}
