// Copyright (c) 2017 Intel Corporation
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

package testing

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containernetworking/cni/pkg/types"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/onsi/gomega"
)

// FakeKubeClient is stub KubeClient for testing
type FakeKubeClient struct {
	pods     map[string]*v1.Pod
	PodCount int
	nets     map[string]string
	NetCount int
}

// NewFakeKubeClient creates FakeKubeClient for testing
func NewFakeKubeClient() *FakeKubeClient {
	return &FakeKubeClient{
		pods: make(map[string]*v1.Pod),
		nets: make(map[string]string),
	}
}

// GetRawWithPath returns k8s raw data from its path
func (f *FakeKubeClient) GetRawWithPath(path string) ([]byte, error) {
	obj, ok := f.nets[path]
	if !ok {
		return nil, fmt.Errorf("resource not found")
	}
	f.NetCount++
	return []byte(obj), nil
}

// AddNetConfig adds net-attach-def into its client
func (f *FakeKubeClient) AddNetConfig(namespace, name, data string) {
	cr := fmt.Sprintf(`{
  "apiVersion": "k8s.cni.cncf.io/v1",
  "kind": "Network",
  "metadata": {
    "namespace": "%s",
    "name": "%s"
  },
  "spec": {
    "config": "%s"
  }
}`, namespace, name, strings.Replace(data, "\"", "\\\"", -1))
	cr = strings.Replace(cr, "\n", "", -1)
	cr = strings.Replace(cr, "\t", "", -1)
	f.nets[fmt.Sprintf("/apis/k8s.cni.cncf.io/v1/namespaces/%s/network-attachment-definitions/%s", namespace, name)] = cr
}

// AddNetConfigAnnotation adds net-attach-def into its client with an annotation
func (f *FakeKubeClient) AddNetConfigAnnotation(namespace, name, data string) {
	cr := fmt.Sprintf(`{
	"apiVersion": "k8s.cni.cncf.io/v1",
	"kind": "Network",
	"metadata": {
	  "namespace": "%s",
	  "name": "%s",
	  "annotations": {
		"k8s.v1.cni.cncf.io/resourceName": "intel.com/sriov"
	  }
	},
	"spec": {
	  "config": "%s"
	}
  }`, namespace, name, strings.Replace(data, "\"", "\\\"", -1))
	cr = strings.Replace(cr, "\n", "", -1)
	cr = strings.Replace(cr, "\t", "", -1)
	f.nets[fmt.Sprintf("/apis/k8s.cni.cncf.io/v1/namespaces/%s/network-attachment-definitions/%s", namespace, name)] = cr
}

// AddNetFile puts config file as net-attach-def
func (f *FakeKubeClient) AddNetFile(namespace, name, filePath, fileData string) {
	cr := fmt.Sprintf(`{
  "apiVersion": "k8s.cni.cncf.io/v1",
  "kind": "Network",
  "metadata": {
    "namespace": "%s",
    "name": "%s"
  }
}`, namespace, name)
	f.nets[fmt.Sprintf("/apis/k8s.cni.cncf.io/v1/namespaces/%s/network-attachment-definitions/%s", namespace, name)] = cr

	err := ioutil.WriteFile(filePath, []byte(fileData), 0600)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// GetPod query pod by namespace/pod and return it if exists
func (f *FakeKubeClient) GetPod(namespace, name string) (*v1.Pod, error) {
	key := fmt.Sprintf("%s/%s", namespace, name)
	pod, ok := f.pods[key]
	if !ok {
		return nil, fmt.Errorf("pod not found")
	}
	f.PodCount++
	return pod, nil
}

// UpdatePodStatus update pod status
func (f *FakeKubeClient) UpdatePodStatus(pod *v1.Pod) (*v1.Pod, error) {
	key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	f.pods[key] = pod
	return f.pods[key], nil
}

// AddPod adds pod into fake client
func (f *FakeKubeClient) AddPod(pod *v1.Pod) {
	key := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
	f.pods[key] = pod
}

// DeletePod remove pod from fake client
func (f *FakeKubeClient) DeletePod(pod *v1.Pod) {
	key := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
	delete(f.pods, key)
}

// NewFakePod creates fake Pod object
func NewFakePod(name string, netAnnotation string, defaultNetAnnotation string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
			UID:       "testUID",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: "ctr1", Image: "image"},
			},
		},
	}
	annotations := make(map[string]string)

	if netAnnotation != "" {
		netAnnotation = strings.Replace(netAnnotation, "\n", "", -1)
		netAnnotation = strings.Replace(netAnnotation, "\t", "", -1)
		annotations["k8s.v1.cni.cncf.io/networks"] = netAnnotation
	}

	if defaultNetAnnotation != "" {
		annotations["v1.multus-cni.io/default-network"] = defaultNetAnnotation
	}

	pod.ObjectMeta.Annotations = annotations
	return pod
}

// EnsureCIDR parses/verify CIDR ip string and convert to net.IPNet
func EnsureCIDR(cidr string) *net.IPNet {
	ip, net, err := net.ParseCIDR(cidr)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	net.IP = ip
	return net
}

// Result is stub Result for testing
type Result struct {
	CNIVersion string             `json:"cniVersion,omitempty"`
	IP4        *types020.IPConfig `json:"ip4,omitempty"`
	IP6        *types020.IPConfig `json:"ip6,omitempty"`
	DNS        types.DNS          `json:"dns,omitempty"`
}

// Version returns current CNIVersion of the given Result
func (r *Result) Version() string {
	return r.CNIVersion
}

// GetAsVersion returns a Result object given a version
func (r *Result) GetAsVersion(version string) (types.Result, error) {
	for _, supportedVersion := range types020.SupportedVersions {
		if version == supportedVersion {
			r.CNIVersion = version
			return r, nil
		}
	}
	return nil, fmt.Errorf("cannot convert version %q to %s", types020.SupportedVersions, version)
}

// Print prints a Result's information to std out
func (r *Result) Print() error {
	return r.PrintTo(os.Stdout)
}

// PrintTo prints a Result's information to the provided writer
func (r *Result) PrintTo(writer io.Writer) error {
	data, err := json.MarshalIndent(r, "", "    ")
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// String returns a formatted string in the form of "[IP4: $1,][ IP6: $2,] DNS: $3" where
// $1 represents the receiver's IPv4, $2 represents the receiver's IPv6 and $3 the
// receiver's DNS. If $1 or $2 are nil, they won't be present in the returned string.
func (r *Result) String() string {
	var str string
	if r.IP4 != nil {
		str = fmt.Sprintf("IP4:%+v, ", *r.IP4)
	}
	if r.IP6 != nil {
		str += fmt.Sprintf("IP6:%+v, ", *r.IP6)
	}
	return fmt.Sprintf("%sDNS:%+v", str, r.DNS)
}
