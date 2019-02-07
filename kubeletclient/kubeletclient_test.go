package kubeletclient

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/kubelet/util"

	mtypes "github.com/intel/multus-cni/types"
	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

var (
	socketDir  string
	socketName string
	fakeServer *fakeResourceServer
)

type fakeResourceServer struct {
	server *grpc.Server
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

func setUp() error {
	tempSocketDir, err := ioutil.TempDir("", "kubelet-resource-client")
	if err != nil {
		return err
	}
	defaultPodResourcesPath = filepath.Join(tempSocketDir, defaultPodResourcesPath)

	if err := os.MkdirAll(defaultPodResourcesPath, os.ModeDir); err != nil {
		return err
	}

	socketDir = defaultPodResourcesPath
	socketName = filepath.Join(socketDir, "kubelet.sock")

	fakeServer = &fakeResourceServer{server: grpc.NewServer()}
	podresourcesapi.RegisterPodResourcesListerServer(fakeServer.server, fakeServer)
	lis, err := util.CreateListener(socketName)
	if err != nil {
		return nil
	}
	go fakeServer.server.Serve(lis)
	return nil
}

func tearDown(path string) error {
	if fakeServer != nil {
		fakeServer.server.Stop()
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return nil
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
			kubeletSocket = socketName
			_, err := GetResourceClient()
			Expect(err).NotTo(HaveOccurred())
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
			kubeletSocket = socketName
			client, err := getKubeletClient()
			Expect(err).NotTo(HaveOccurred())

			outputRMap := map[string]*mtypes.ResourceInfo{
				"resource": &mtypes.ResourceInfo{DeviceIDs: []string{"dev0", "dev1"}},
			}
			resourceMap, err := client.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceMap).ShouldNot(BeNil())
			Expect(resourceMap).To(Equal(outputRMap))
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
			kubeletSocket = socketName
			client, err := getKubeletClient()
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
			kubeletSocket = socketName
			client, err := getKubeletClient()
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

			kubeletSocket = socketName
			client, err := getKubeletClient()
			Expect(err).NotTo(HaveOccurred())

			emptyRMap := map[string]*mtypes.ResourceInfo{}
			resourceMap, err := client.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceMap).ShouldNot(BeNil())
			Expect(resourceMap).To(Equal(emptyRMap))
		})
	})
})
