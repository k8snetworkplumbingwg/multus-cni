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
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/checkpoint"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	v1 "k8s.io/api/core/v1"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

const (
	defaultKubeletSocket       = "kubelet" // which is defined in k8s.io/kubernetes/pkg/kubelet/apis/podresources
	kubeletConnectionTimeout   = 10 * time.Second
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
	defaultPodResourcesPath    = "/var/lib/kubelet/pod-resources"
	unixProtocol               = "unix"
)

// LocalEndpoint returns the full path to a unix socket at the given endpoint
// which is in k8s.io/kubernetes/pkg/kubelet/util
func localEndpoint(path string) *url.URL {
	return &url.URL{
		Scheme: unixProtocol,
		Path:   path + ".sock",
	}
}

// GetResourceClient returns an instance of ResourceClient interface initialized with Pod resource information
func GetResourceClient(kubeletSocket string) (types.ResourceClient, error) {
	kubeletSocketURL := localEndpoint(filepath.Join(defaultPodResourcesPath, defaultKubeletSocket))

	if kubeletSocket != "" {
		kubeletSocketURL = &url.URL{
			Scheme: unixProtocol,
			Path:   kubeletSocket,
		}
	}
	// If Kubelet resource API endpoint exist use that by default
	// Or else fallback with checkpoint file
	if hasKubeletAPIEndpoint(kubeletSocketURL) {
		logging.Debugf("GetResourceClient: using Kubelet resource API endpoint")
		return getKubeletClient(kubeletSocketURL)
	}

	logging.Debugf("GetResourceClient: using Kubelet device plugin checkpoint")
	return checkpoint.GetCheckpoint()
}

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, unixProtocol, addr)
}

func getKubeletResourceClient(kubeletSocketURL *url.URL, timeout time.Duration) (podresourcesapi.PodResourcesListerClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, kubeletSocketURL.Path, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dial),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaultPodResourcesMaxSize)))
	if err != nil {
		return nil, nil, fmt.Errorf("error dialing socket %s: %v", kubeletSocketURL.Path, err)
	}
	return podresourcesapi.NewPodResourcesListerClient(conn), conn, nil
}

func getKubeletClient(kubeletSocketURL *url.URL) (types.ResourceClient, error) {
	newClient := &kubeletClient{}

	client, conn, err := getKubeletResourceClient(kubeletSocketURL, 10*time.Second)
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
		return nil, logging.Errorf("GetPodResourceMap: Pod name or namespace cannot be empty")
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

func hasKubeletAPIEndpoint(url *url.URL) bool {
	// Check for kubelet resource API socket file
	if _, err := os.Stat(url.Path); err != nil {
		logging.Debugf("hasKubeletAPIEndpoint: error looking up kubelet resource api socket file: %q", err)
		return false
	}
	return true
}
