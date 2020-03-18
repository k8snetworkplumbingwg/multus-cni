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

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containernetworking/cni/pkg/types"
	types020 "github.com/containernetworking/cni/pkg/types/020"

	"github.com/onsi/gomega"
)

// NewFakeNetAttachDef returns net-attach-def for testing
func NewFakeNetAttachDef(namespace, name, config string) *netv1.NetworkAttachmentDefinition {
	return &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: netv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
}

// NewFakeNetAttachDefFile returns net-attach-def for testing with conf file
func NewFakeNetAttachDefFile(namespace, name, filePath, fileData string) *netv1.NetworkAttachmentDefinition {
	netAttach := &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := ioutil.WriteFile(filePath, []byte(fileData), 0600)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return netAttach
}

// NewFakeNetAttachDefAnnotation returns net-attach-def with resource annotation
func NewFakeNetAttachDefAnnotation(namespace, name, config string) *netv1.NetworkAttachmentDefinition {
	return &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/resourceName": "intel.com/sriov",
			},
		},
		Spec: netv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
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
