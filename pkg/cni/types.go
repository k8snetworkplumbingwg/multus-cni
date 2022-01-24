package cni

import (
	"net/http"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types/current"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

type cniRequestFunc func(request *PodRequest) ([]byte, error)

// Explicit type for CNI commands the server handles
type command string

// PodRequest represents a request for ADD / DEL / CHECK for a Pod
type PodRequest struct {
	// The CNI command of the operation
	Command command

	// embed the Kubernetes runtime args
	types.K8sArgs

	// embed the CniArgs
	skel.CmdArgs

	kubeClient *k8sclient.ClientInfo

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

// Response represents the response (computed in the CNI server) for
// ADD / DEL / CHECK for a Pod.
type Response struct {
	Result *current.Result
}
