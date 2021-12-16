package cni

import (
	"context"
	"github.com/containernetworking/cni/pkg/invoke"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	"net/http"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
)

type cniRequestFunc func(request *PodRequest) ([]byte, error)

// Explicit type for CNI commands the server handles
type command string

// PodRequest represents a request for ADD / DEL / CHECK for a Pod
type PodRequest struct {
	// The CNI command of the operation
	Command command
	// kubernetes namespace name
	Namespace string
	// kubernetes pod name
	Name string
	// kubernetes pod UID
	UID string
	// kubernetes container ID
	SandboxID string
	// CNI container ID
	ContainerID string
	// kernel network namespace path
	Netns string
	// Interface name to be configured
	IfName string
	// CNI conf obtained from stdin conf
	CNIConf *types.NetConf
	// Timestamp when the request was started
	timestamp time.Time
	// ctx is a context tracking this request's lifetime
	ctx context.Context
	// cancel should be called to cancel this request
	cancel context.CancelFunc
	// kubeclient has the kubernetes API client
	kubeclient *k8sclient.ClientInfo
	// exec execs
	exec invoke.Exec
}

// Request sent to the Server by the multus-shim
type Request struct {
	// CNI environment variables, like CNI_COMMAND and CNI_NETNS
	Env map[string]string `json:"env,omitempty"`
	// CNI configuration passed via stdin to the CNI plugin
	Config []byte `json:"config,omitempty"`
}

// Server represents an HTTP server listening to a unix socket. It will handle
// the CNI shim requests issued when a pod is added / removed.
type Server struct {
	http.Server
	requestFunc cniRequestFunc
	rundir      string
	kubeclient  *k8sclient.ClientInfo
	exec        invoke.Exec
}

// Plugin represents the connection between the CNI shim and server.
type Plugin struct {
	SocketPath string
}

// Response represents the response (computed in the CNI server) for
// ADD / DEL / CHECK for a Pod.
type Response struct {
	Result *current.Result
}
