package server

import (
	"net/http"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cni100 "github.com/containernetworking/cni/pkg/types/100"

	"github.com/prometheus/client_golang/prometheus"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

type cniRequestFunc func(cmd string, k8sArgs *types.K8sArgs, cniCmdArgs *skel.CmdArgs, exec invoke.Exec, kubeClient *k8sclient.ClientInfo) ([]byte, error)

// Request sent to the Server by the multus-shim
type Request struct {
	// CNI environment variables, like CNI_COMMAND and CNI_NETNS
	Env map[string]string `json:"env,omitempty"`
	// CNI configuration passed via stdin to the CNI plugin
	Config []byte `json:"config,omitempty"`
}

// Metrics represents server's metrics.
type Metrics struct {
	requestCounter *prometheus.CounterVec
}

// Server represents an HTTP server listening to a unix socket. It will handle
// the CNI shim requests issued when a pod is added / removed.
type Server struct {
	http.Server
	requestFunc  cniRequestFunc
	rundir       string
	kubeclient   *k8sclient.ClientInfo
	exec         invoke.Exec
	serverConfig []byte
	metrics      *Metrics
}

// Response represents the response (computed in the CNI server) for
// ADD / DEL / CHECK for a Pod.
type Response struct {
	Result *cni100.Result
}

// ShimNetConf for the shim cni config file written in json
type ShimNetConf struct {
	types.NetConf

	MultusSocketDir string `json:"socketDir"`
	LogFile         string `json:"logFile,omitempty"`
	LogLevel        string `json:"logLevel,omitempty"`
	LogToStderr     bool   `json:"logToStderr,omitempty"`
}

// ControllerNetConf for the controller cni configuration
type ControllerNetConf struct {
	ConfDir     string `json:"confDir"`
	CNIDir      string `json:"cniDir"`
	BinDir      string `json:"binDir"`
	LogFile     string `json:"logFile"`
	LogLevel    string `json:"logLevel"`
	LogToStderr bool   `json:"logToStderr,omitempty"`

	// Option to point to the path of the unix domain socket through which the
	// multus client / server communicate.
	MultusSocketDir string `json:"socketDir"`
}
