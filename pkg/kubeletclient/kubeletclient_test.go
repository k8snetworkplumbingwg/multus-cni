// Copyright (c) 2019 Intel Corporation
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

package kubeletclient

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	mtypes "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

var (
	socketDir  string
	socketName string
	fakeServer *fakeResourceServer
)

type fakeResourceServer struct {
	server *grpc.Server
}

//TODO: This is stub code for test, but we may need to change for the testing we use this API in the future...
func (m *fakeResourceServer) GetAllocatableResources(ctx context.Context, req *podresourcesapi.AllocatableResourcesRequest) (*podresourcesapi.AllocatableResourcesResponse, error) {
	return &podresourcesapi.AllocatableResourcesResponse{}, nil
}

func (m *fakeResourceServer) List(ctx context.Context, req *podresourcesapi.ListPodResourcesRequest) (*podresourcesapi.ListPodResourcesResponse, error) {
	podName := "pod-name"
	podNamespace := "pod-namespace"
	containerName := "container-name"

	devs := []*podresourcesapi.ContainerDevices{
		{
			ResourceName: "resource",
			DeviceIds:    []string{"dev0", "dev1"},
		},
	}

	resp := &podresourcesapi.ListPodResourcesResponse{
		PodResources: []*podresourcesapi.PodResources{
			{
				Name:      podName,
				Namespace: podNamespace,
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name:    containerName,
						Devices: devs,
					},
				},
			},
		},
	}
	return resp, nil
}

func TestKubeletclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubeletclient Suite")
}

var testKubeletSocket string

// CreateListener creates a listener on the specified endpoint.
// based from k8s.io/kubernetes/pkg/kubelet/util
func CreateListener(addr string) (net.Listener, error) {
	// Unlink to cleanup the previous socket file.
	err := unix.Unlink(addr)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to unlink socket file %q: %v", addr, err)
	}

	if err := os.MkdirAll(filepath.Dir(addr), 0750); err != nil {
		return nil, fmt.Errorf("error creating socket directory %q: %v", filepath.Dir(addr), err)
	}

	// Create the socket on a tempfile and move it to the destination socket to handle improper cleanup
	file, err := os.CreateTemp(filepath.Dir(addr), "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %v", err)
	}

	if err := os.Remove(file.Name()); err != nil {
		return nil, fmt.Errorf("failed to remove temporary file: %v", err)
	}

	l, err := net.Listen(unixProtocol, file.Name())
	if err != nil {
		return nil, err
	}

	if err = os.Rename(file.Name(), addr); err != nil {
		return nil, fmt.Errorf("failed to move temporary file to addr %q: %v", addr, err)
	}

	return l, nil
}

func setUp() error {
	tempSocketDir, err := os.MkdirTemp("", "kubelet-resource-client")
	if err != nil {
		return err
	}
	testingPodResourcesPath := filepath.Join(tempSocketDir, defaultPodResourcesPath)

	if err := os.MkdirAll(testingPodResourcesPath, os.ModeDir); err != nil {
		return err
	}

	socketDir = testingPodResourcesPath
	socketName = filepath.Join(socketDir, "kubelet.sock")
	testKubeletSocket, _ = LocalEndpoint(socketDir, "kubelet")

	fakeServer = &fakeResourceServer{server: grpc.NewServer()}
	podresourcesapi.RegisterPodResourcesListerServer(fakeServer.server, fakeServer)
	lis, err := CreateListener(socketName)
	if err != nil {
		return err
	}
	go fakeServer.server.Serve(lis)
	return nil
}

func tearDown(path string) error {
	if fakeServer != nil {
		fakeServer.server.Stop()
	}
	err := os.RemoveAll(path)
	return err
}

var _ = BeforeSuite(func() {
	err := setUp()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := tearDown(socketDir)
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Kubelet resource endpoint data read operations", func() {

	Context("GetResourceClient()", func() {
		It("should return no error", func() {
			_, err := GetResourceClient(testKubeletSocket)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail with missing file", func() {
			_, err := GetResourceClient("unix:/sampleSocketString")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error reading file"))
		})

		It("should fail with invalid protocol", func() {
			_, err := GetResourceClient("tcp:" + socketName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only support unix socket endpoint"))
		})
	})
	Context("GetPodResourceMap() with valid pod name and namespace", func() {
		It("should return no error", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-name",
					Namespace: "pod-namespace",
					UID:       podUID,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "container-name",
						},
					},
				},
			}
			client, err := getKubeletClient(testKubeletSocket)
			Expect(err).NotTo(HaveOccurred())

			outputRMap := map[string]*mtypes.ResourceInfo{
				"resource": {DeviceIDs: []string{"dev0", "dev1"}},
			}
			resourceMap, err := client.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceMap).ShouldNot(BeNil())
			Expect(resourceMap).To(Equal(outputRMap))
		})

		It("should return an error with garbage socket value", func() {
			_, err := getKubeletClient("/badfilepath!?//")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetPodResourceMap() with empty podname", func() {
		It("should return error", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "pod-namespace",
					UID:       podUID,
				},
			}
			client, err := getKubeletClient(testKubeletSocket)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.GetPodResourceMap(fakePod)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetPodResourceMap() with empty namespace", func() {
		It("should return error", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-name",
					Namespace: "",
					UID:       podUID,
				},
			}
			client, err := getKubeletClient(testKubeletSocket)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.GetPodResourceMap(fakePod)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetPodResourceMap() with non-existent podname and namespace", func() {
		It("should return no error", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whateverpod",
					Namespace: "whatevernamespace",
					UID:       podUID,
				},
			}

			client, err := getKubeletClient(testKubeletSocket)
			Expect(err).NotTo(HaveOccurred())

			emptyRMap := map[string]*mtypes.ResourceInfo{}
			resourceMap, err := client.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceMap).ShouldNot(BeNil())
			Expect(resourceMap).To(Equal(emptyRMap))
		})
	})
})
