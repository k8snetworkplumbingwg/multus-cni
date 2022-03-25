package crio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/pkg/errors"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes/crio/types"
	crioruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/util"
)

// CrioRuntime represents a connection to the CRI-O runtime
type CrioRuntime struct {
	cancelFunc context.CancelFunc
	client     crioruntime.RuntimeServiceClient
	context    context.Context
}

// NewCrioRuntime returns a connection to the CRI-O runtime
func NewCrioRuntime(socketPath string, timeout time.Duration) (*CrioRuntime, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("path to cri-o socket missing")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	clientConnection, err := getConnection([]string{socketPath})
	if err != nil {
		cancelFunc()
		return nil, errors.Wrap(err, "connect")
	}
	runtimeClient := crioruntime.NewRuntimeServiceClient(clientConnection)

	return &CrioRuntime{
		client:     runtimeClient,
		context:    ctx,
		cancelFunc: cancelFunc,
	}, nil
}

func getConnection(endPoints []string) (*grpc.ClientConn, error) {
	if endPoints == nil || len(endPoints) == 0 {
		return nil, fmt.Errorf("endpoint is not set")
	}
	endPointsLen := len(endPoints)
	var conn *grpc.ClientConn
	for i, endPoint := range endPoints {
		addr, dialer, err := util.GetAddressAndDialer(endPoint)
		if err != nil {
			if i == endPointsLen-1 {
				return nil, err
			}
			continue
		}
		conn, err = grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second), grpc.WithContextDialer(dialer))
		if err != nil {
			errMsg := errors.Wrapf(err, "connect endpoint '%s', make sure you are running as root and the endpoint has been started", endPoint)
			if i == endPointsLen-1 {
				return nil, errMsg
			}
		} else {
			break
		}
	}
	return conn, nil
}

// NetNS returns the network namespace of the given containerID.
func (cr *CrioRuntime) NetNS(containerID string) (string, error) {
	reply, err := cr.client.ContainerStatus(context.Background(), &crioruntime.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get pod sandbox info: %w", err) //errors.Wrap(err, "failed to get pod sandbox info")
	}

	podStatusResponseInfo, err := ContainerStatus(reply)
	if err != nil {
		return "", err
	}

	namespaces := podStatusResponseInfo.RunTimeSpec.Linux.Namespaces
	for _, namespace := range namespaces {
		if namespace.Type == "network" {
			return namespace.Path, nil
		}
	}
	return "", nil
}

func ContainerStatus(reply *crioruntime.ContainerStatusResponse) (types.PodStatusResponseInfo, error) {
	var podStatusResponseInfo types.PodStatusResponseInfo
	info := reply.GetInfo()["info"]
	if err := json.Unmarshal([]byte(info), &podStatusResponseInfo); err != nil {
		if e, ok := err.(*json.SyntaxError); ok {
			return types.PodStatusResponseInfo{}, fmt.Errorf("error unmarshalling cri-o's response: syntax error at byte offset %d. Error: %w", e.Offset, e)
		}
		return types.PodStatusResponseInfo{}, err
	}
	return podStatusResponseInfo, nil
}
