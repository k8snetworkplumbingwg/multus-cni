package kubeletclient

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/intel/multus-cni.v3/pkg/checkpoint"
	"gopkg.in/intel/multus-cni.v3/pkg/logging"
	"gopkg.in/intel/multus-cni.v3/pkg/types"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/util"
)

const (
	defaultKubeletSocketFile   = "kubelet.sock"
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
)

var (
	kubeletSocket           string
	defaultPodResourcesPath = "/var/lib/kubelet/pod-resources"
)

// GetResourceClient returns an instance of ResourceClient interface initialized with Pod resource information
func GetResourceClient() (types.ResourceClient, error) {
	// If Kubelet resource API endpoint exist use that by default
	// Or else fallback with checkpoint file
	if hasKubeletAPIEndpoint() {
		logging.Debugf("GetResourceClient: using Kubelet resource API endpoint")
		return getKubeletClient()
	}

	logging.Debugf("GetResourceClient: using Kubelet device plugin checkpoint")
	return checkpoint.GetCheckpoint()
}

func getKubeletClient() (types.ResourceClient, error) {
	newClient := &kubeletClient{}
	if kubeletSocket == "" {
		kubeletSocket = util.LocalEndpoint(defaultPodResourcesPath, podresources.Socket)
	}

	client, conn, err := podresources.GetClient(kubeletSocket, 10*time.Second, defaultPodResourcesMaxSize)
	if err != nil {
		return nil, logging.Errorf("getKubeletClient: error getting grpc client: %v\n", err)
	}
	defer conn.Close()

	if err := newClient.getPodResources(client); err != nil {
		return nil, logging.Errorf("getKubeletClient: error ge tting pod resources from client: %v\n", err)
	}

	return newClient, nil
}

type kubeletClient struct {
	resources []*podresourcesapi.PodResources
}

func (rc *kubeletClient) getPodResources(client podresourcesapi.PodResourcesListerClient) error {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		return logging.Errorf("getPodResources: failed to list pod resources, %v.Get(_) = _, %v", client, err)
	}

	rc.resources = resp.PodResources
	return nil
}

// GetPodResourceMap returns an instance of a map of Pod ResourceInfo given a (Pod name, namespace) tuple
func (rc *kubeletClient) GetPodResourceMap(pod *v1.Pod) (map[string]*types.ResourceInfo, error) {
	resourceMap := make(map[string]*types.ResourceInfo)

	name := pod.Name
	ns := pod.Namespace

	if name == "" || ns == "" {
		return nil, logging.Errorf("GetPodResourcesMap: Pod name or namespace cannot be empty")
	}

	for _, pr := range rc.resources {
		if pr.Name == name && pr.Namespace == ns {
			for _, cnt := range pr.Containers {
				for _, dev := range cnt.Devices {
					if rInfo, ok := resourceMap[dev.ResourceName]; ok {
						rInfo.DeviceIDs = append(rInfo.DeviceIDs, dev.DeviceIds...)
					} else {
						resourceMap[dev.ResourceName] = &types.ResourceInfo{DeviceIDs: dev.DeviceIds}
					}
				}
			}
		}
	}
	return resourceMap, nil
}

func hasKubeletAPIEndpoint() bool {
	// Check for kubelet resource API socket file
	kubeletAPISocket := filepath.Join(defaultPodResourcesPath, defaultKubeletSocketFile)
	if _, err := os.Stat(kubeletAPISocket); err != nil {
		logging.Debugf("hasKubeletAPIEndpoint: error looking up kubelet resource api socket file: %q", err)
		return false
	}
	return true
}
