package fake

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"

	crioruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes/crio/types"
)

type CrioClient struct {
	cache map[string]string
}

type ClientOpt func(client *CrioClient)

func NewFakeClient(opts ...ClientOpt) *CrioClient {
	client := &CrioClient{cache: map[string]string{}}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func WithCachedContainer(containerID string, netnsPath string) ClientOpt {
	return func(client *CrioClient) {
		client.cache[containerID] = netnsPath
	}
}

func (CrioClient) Version(ctx context.Context, in *crioruntime.VersionRequest, opts ...grpc.CallOption) (*crioruntime.VersionResponse, error) {
	return nil, nil
}

func (CrioClient) RunPodSandbox(ctx context.Context, in *crioruntime.RunPodSandboxRequest, opts ...grpc.CallOption) (*crioruntime.RunPodSandboxResponse, error) {
	return nil, nil
}

func (CrioClient) StopPodSandbox(ctx context.Context, in *crioruntime.StopPodSandboxRequest, opts ...grpc.CallOption) (*crioruntime.StopPodSandboxResponse, error) {
	return nil, nil
}

func (CrioClient) RemovePodSandbox(ctx context.Context, in *crioruntime.RemovePodSandboxRequest, opts ...grpc.CallOption) (*crioruntime.RemovePodSandboxResponse, error) {
	return nil, nil
}

func (CrioClient) PodSandboxStatus(ctx context.Context, in *crioruntime.PodSandboxStatusRequest, opts ...grpc.CallOption) (*crioruntime.PodSandboxStatusResponse, error) {
	return nil, nil
}

func (CrioClient) ListPodSandbox(ctx context.Context, in *crioruntime.ListPodSandboxRequest, opts ...grpc.CallOption) (*crioruntime.ListPodSandboxResponse, error) {
	return nil, nil
}

func (CrioClient) CreateContainer(ctx context.Context, in *crioruntime.CreateContainerRequest, opts ...grpc.CallOption) (*crioruntime.CreateContainerResponse, error) {
	return nil, nil
}

func (CrioClient) StartContainer(ctx context.Context, in *crioruntime.StartContainerRequest, opts ...grpc.CallOption) (*crioruntime.StartContainerResponse, error) {
	return nil, nil
}

func (CrioClient) StopContainer(ctx context.Context, in *crioruntime.StopContainerRequest, opts ...grpc.CallOption) (*crioruntime.StopContainerResponse, error) {
	return nil, nil
}

func (CrioClient) RemoveContainer(ctx context.Context, in *crioruntime.RemoveContainerRequest, opts ...grpc.CallOption) (*crioruntime.RemoveContainerResponse, error) {
	return nil, nil
}

func (CrioClient) ListContainers(ctx context.Context, in *crioruntime.ListContainersRequest, opts ...grpc.CallOption) (*crioruntime.ListContainersResponse, error) {
	return nil, nil
}

func (cc CrioClient) ContainerStatus(ctx context.Context, in *crioruntime.ContainerStatusRequest, opts ...grpc.CallOption) (*crioruntime.ContainerStatusResponse, error) {
	containerId := in.ContainerId
	netnsPath, wasFound := cc.cache[containerId]
	if !wasFound {
		return nil, fmt.Errorf("container %s not found", containerId)
	}

	containerStatus := types.PodStatusResponseInfo{
		RunTimeSpec: types.RunTimeSpecInfo{
			Linux: types.NamespacesInfo{
				Namespaces: []types.NameSpaceInfo{
					{
						Type: "network",
						Path: netnsPath,
					},
				},
			},
		},
	}

	marshalledContainerStatus, err := json.Marshal(&containerStatus)
	if err != nil {
		return nil, fmt.Errorf("error marshalling the container status: %v", err)
	}

	return &crioruntime.ContainerStatusResponse{
		Info: map[string]string{"info": string(marshalledContainerStatus)},
	}, nil
}

func (CrioClient) UpdateContainerResources(ctx context.Context, in *crioruntime.UpdateContainerResourcesRequest, opts ...grpc.CallOption) (*crioruntime.UpdateContainerResourcesResponse, error) {
	return nil, nil
}

func (CrioClient) ReopenContainerLog(ctx context.Context, in *crioruntime.ReopenContainerLogRequest, opts ...grpc.CallOption) (*crioruntime.ReopenContainerLogResponse, error) {
	return nil, nil
}

func (CrioClient) ExecSync(ctx context.Context, in *crioruntime.ExecSyncRequest, opts ...grpc.CallOption) (*crioruntime.ExecSyncResponse, error) {
	return nil, nil
}

func (CrioClient) Exec(ctx context.Context, in *crioruntime.ExecRequest, opts ...grpc.CallOption) (*crioruntime.ExecResponse, error) {
	return nil, nil
}

func (CrioClient) Attach(ctx context.Context, in *crioruntime.AttachRequest, opts ...grpc.CallOption) (*crioruntime.AttachResponse, error) {
	return nil, nil
}

func (CrioClient) PortForward(ctx context.Context, in *crioruntime.PortForwardRequest, opts ...grpc.CallOption) (*crioruntime.PortForwardResponse, error) {
	return nil, nil
}

func (CrioClient) ContainerStats(ctx context.Context, in *crioruntime.ContainerStatsRequest, opts ...grpc.CallOption) (*crioruntime.ContainerStatsResponse, error) {
	return nil, nil
}

func (CrioClient) ListContainerStats(ctx context.Context, in *crioruntime.ListContainerStatsRequest, opts ...grpc.CallOption) (*crioruntime.ListContainerStatsResponse, error) {
	return nil, nil
}

func (CrioClient) UpdateRuntimeConfig(ctx context.Context, in *crioruntime.UpdateRuntimeConfigRequest, opts ...grpc.CallOption) (*crioruntime.UpdateRuntimeConfigResponse, error) {
	return nil, nil
}

func (CrioClient) Status(ctx context.Context, in *crioruntime.StatusRequest, opts ...grpc.CallOption) (*crioruntime.StatusResponse, error) {
	return nil, nil
}
