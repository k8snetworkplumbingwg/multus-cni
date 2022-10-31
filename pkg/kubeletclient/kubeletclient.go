package kubeletclient

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/checkpoint"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	v1 "k8s.io/api/core/v1"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

const (
	defaultKubeletSocket       = "kubelet" // which is defined in k8s.io/kubernetes/pkg/kubelet/apis/podresources
	kubeletConnectionTimeout   = 10 * time.Second
	defaultKubeletSocketFile   = "kubelet.sock"
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
	defaultPodResourcesPath    = "/var/lib/kubelet/pod-resources"
	unixProtocol               = "unix"
)

// LocalEndpoint returns the full path to a unix socket at the given endpoint
// which is in k8s.io/kubernetes/pkg/kubelet/util
func LocalEndpoint(path, file string) (string, error) {
	u := url.URL{
		Scheme: unixProtocol,
		Path:   path,
	}
	return filepath.Join(u.String(), file+".sock"), nil
}

func removeUnixProtocol(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme != unixProtocol {
		return "", fmt.Errorf("only support unix socket endpoint")
	}
	return u.Path, nil
}

// GetResourceClient returns an instance of ResourceClient interface initialized with Pod resource information
func GetResourceClient(kubeletSocket string) (types.ResourceClient, error) {
	if kubeletSocket == "" {
		kubeletSocket, _ = LocalEndpoint(defaultPodResourcesPath, defaultKubeletSocket)
	}
	// If Kubelet resource API endpoint exist use that by default
	// Or else fallback with checkpoint file
	if hasKubeletAPIEndpoint(kubeletSocket) {
		logging.Debugf("GetResourceClient: using Kubelet resource API endpoint")
		return getKubeletClient(kubeletSocket)
	}

	logging.Debugf("GetResourceClient: using Kubelet device plugin checkpoint")
	return checkpoint.GetCheckpoint()
}

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, unixProtocol, addr)
}

func getKubeletResourceClient(kubeletSocket string, timeout time.Duration) (podresourcesapi.PodResourcesListerClient, *grpc.ClientConn, error) {
	addr, err := removeUnixProtocol(kubeletSocket)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dial),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaultPodResourcesMaxSize)))
	if err != nil {
		return nil, nil, fmt.Errorf("error dialing socket %s: %v", kubeletSocket, err)
	}
	return podresourcesapi.NewPodResourcesListerClient(conn), conn, nil
}

func getKubeletClient(kubeletSocket string) (types.ResourceClient, error) {
	newClient := &kubeletClient{}
	if kubeletSocket == "" {
		kubeletSocket, _ = LocalEndpoint(defaultPodResourcesPath, defaultKubeletSocket)
	}

	client, conn, err := getKubeletResourceClient(kubeletSocket, 10*time.Second)
	if err != nil {
		return nil, logging.Errorf("getKubeletClient: error getting grpc client: %v\n", err)
	}
	defer conn.Close()

	if err := newClient.getPodResources(client); err != nil {
		return nil, logging.Errorf("getKubeletClient: error getting pod resources from client: %v\n", err)
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

func hasKubeletAPIEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	// Check for kubelet resource API socket file
	if _, err := os.Stat(u.Path); err != nil {
		logging.Debugf("hasKubeletAPIEndpoint: error looking up kubelet resource api socket file: %q", err)
		return false
	}
	return true
}
