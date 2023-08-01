// Copyright (c) 2022 Multus Authors
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

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	k8s "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/api"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/config"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const (
	fullReadWriteExecutePermissions    = 0777
	thickPluginSocketRunDirPermissions = 0700
)

// FilesystemPreRequirements ensures the target `rundir` features the correct
// permissions.
func FilesystemPreRequirements(rundir string) error {
	if err := os.RemoveAll(rundir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old pod info socket directory %s: %v", rundir, err)
	}
	if err := os.MkdirAll(rundir, thickPluginSocketRunDirPermissions); err != nil {
		return fmt.Errorf("failed to create pod info socket directory %s: %v", rundir, err)
	}
	return nil
}

func printCmdArgs(args *skel.CmdArgs) string {
	return fmt.Sprintf("ContainerID:%q Netns:%q IfName:%q Args:%q Path:%q",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)
}

// HandleCNIRequest is the CNI server handler function; it is invoked whenever
// a CNI request is processed.
func (s *Server) HandleCNIRequest(cmd string, k8sArgs *types.K8sArgs, cniCmdArgs *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) ([]byte, error) {
	var result []byte
	var err error

	logging.Verbosef("%s starting CNI request %s", cmd, printCmdArgs(cniCmdArgs))
	switch cmd {
	case "ADD":
		result, err = cmdAdd(cniCmdArgs, k8sArgs, exec, kubeClient)
	case "DEL":
		err = cmdDel(cniCmdArgs, k8sArgs, exec, kubeClient)
	case "CHECK":
		err = cmdCheck(cniCmdArgs, k8sArgs, exec, kubeClient)
	default:
		return []byte(""), fmt.Errorf("unknown cmd type: %s", cmd)
	}
	logging.Verbosef("%s finished CNI request %s, result: %q, err: %v", cmd, printCmdArgs(cniCmdArgs), string(result), err)
	if err != nil {
		// Prefix errors with request info for easier failure debugging
		return nil, fmt.Errorf("%s ERRORED: %v", printCmdArgs(cniCmdArgs), err)
	}
	return result, nil
}

// HandleDelegateRequest is the CNI server handler function; it is invoked whenever
// a CNI request is processed as delegate CNI request.
func (s *Server) HandleDelegateRequest(cmd string, k8sArgs *types.K8sArgs, cniCmdArgs *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo, interfaceAttributes *api.DelegateInterfaceAttributes) ([]byte, error) {
	var result []byte
	var err error
	var multusConfByte []byte

	multusConfByte = bytes.Replace(s.serverConfig, []byte(","), []byte("{"), 1)
	multusConfig := types.GetDefaultNetConf()
	if err = json.Unmarshal(multusConfByte, multusConfig); err != nil {
		return nil, err
	}

	logging.Verbosef("%s starting delegate request %s", cmd, printCmdArgs(cniCmdArgs))
	switch cmd {
	case "ADD":
		result, err = cmdDelegateAdd(cniCmdArgs, k8sArgs, exec, kubeClient, multusConfig, interfaceAttributes)
	case "DEL":
		err = cmdDelegateDel(cniCmdArgs, k8sArgs, exec, kubeClient, multusConfig)
	case "CHECK":
		err = cmdDelegateCheck(cniCmdArgs, k8sArgs, exec, kubeClient, multusConfig)
	default:
		return []byte(""), fmt.Errorf("unknown cmd type: %s", cmd)
	}
	logging.Verbosef("%s finished Delegate request %s, result: %q, err: %v", cmd, printCmdArgs(cniCmdArgs), string(result), err)
	if err != nil {
		// Prefix errors with request info for easier failure debugging
		return nil, fmt.Errorf("%s ERRORED: %v", printCmdArgs(cniCmdArgs), err)
	}
	return result, nil
}

// GetListener creates a listener to a unix socket located in `socketPath`
func GetListener(socketPath string) (net.Listener, error) {
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, logging.Errorf("failed to listen on pod info socket: %v", err)
	}
	if err := os.Chmod(socketPath, config.UserRWPermission); err != nil {
		_ = l.Close()
		return nil, logging.Errorf("failed to listen on pod info socket: %v", err)
	}
	return l, nil
}

// NewCNIServer creates and returns a new Server object which will listen on a socket in the given path
func NewCNIServer(daemonConfig *ControllerNetConf, serverConfig []byte) (*Server, error) {
	kubeClient, err := k8s.InClusterK8sClient()
	if err != nil {
		return nil, fmt.Errorf("error getting k8s client: %v", err)
	}

	exec := invoke.Exec(nil)
	if daemonConfig.ChrootDir != "" {
		chrootExec := &ChrootExec{
			Stderr:    os.Stderr,
			chrootDir: daemonConfig.ChrootDir,
		}
		types.ChrootMutex = &chrootExec.mu
		exec = chrootExec
		logging.Verbosef("server configured with chroot: %s", daemonConfig.ChrootDir)
	}

	return newCNIServer(daemonConfig.SocketDir, kubeClient, exec, serverConfig)
}

func newCNIServer(rundir string, kubeClient *k8s.ClientInfo, exec invoke.Exec, servConfig []byte) (*Server, error) {

	// preprocess server config to be used to override multus CNI config
	// see extractCniData() for the detail
	if servConfig != nil {
		servConfig = bytes.Replace(servConfig, []byte("{"), []byte(","), 1)
	}

	router := http.NewServeMux()
	s := &Server{
		Server: http.Server{
			Handler: router,
		},
		rundir:       rundir,
		kubeclient:   kubeClient,
		exec:         exec,
		serverConfig: servConfig,
		metrics: &Metrics{
			requestCounter: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "multus_server_request_total",
					Help: "Counter of HTTP requests",
				},
				[]string{"handler", "code", "method"},
			),
		},
	}
	// register metrics
	prometheus.MustRegister(s.metrics.requestCounter)

	// handle for '/cni'
	router.HandleFunc(api.MultusCNIAPIEndpoint, promhttp.InstrumentHandlerCounter(s.metrics.requestCounter.MustCurryWith(prometheus.Labels{"handler": api.MultusCNIAPIEndpoint}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, fmt.Sprintf("Method not allowed"), http.StatusMethodNotAllowed)
				return
			}

			result, err := s.handleCNIRequest(r)
			if err != nil {
				http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			// Empty response JSON means success with no body
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(result); err != nil {
				_ = logging.Errorf("Error writing HTTP response: %v", err)
			}
		})))

	// handle for '/delegate'
	router.HandleFunc(api.MultusDelegateAPIEndpoint, promhttp.InstrumentHandlerCounter(s.metrics.requestCounter.MustCurryWith(prometheus.Labels{"handler": api.MultusDelegateAPIEndpoint}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, fmt.Sprintf("Method not allowed"), http.StatusMethodNotAllowed)
				return
			}

			result, err := s.handleDelegateRequest(r)
			if err != nil {
				http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			// Empty response JSON means success with no body
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(result); err != nil {
				_ = logging.Errorf("Error writing HTTP response: %v", err)
			}
		})))

	// handle for '/healthz'
	router.HandleFunc(api.MultusHealthAPIEndpoint, promhttp.InstrumentHandlerCounter(s.metrics.requestCounter.MustCurryWith(prometheus.Labels{"handler": api.MultusHealthAPIEndpoint}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				http.Error(w, fmt.Sprintf("Method not allowed"), http.StatusMethodNotAllowed)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
		})))

	// this handle for the rest of above
	router.HandleFunc("/", promhttp.InstrumentHandlerCounter(s.metrics.requestCounter.MustCurryWith(prometheus.Labels{"handler": "NotFound"}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = logging.Errorf("http not found: %v", r)
			w.WriteHeader(http.StatusNotFound)
		})))

	return s, nil
}

func (s *Server) handleCNIRequest(r *http.Request) ([]byte, error) {
	var cr api.Request
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, err
	}
	cmdType, cniCmdArgs, err := extractCniData(&cr, s.serverConfig)
	if err != nil {
		return nil, fmt.Errorf("could not extract the CNI command args: %w", err)
	}

	k8sArgs, err := kubernetesRuntimeArgs(cr.Env, s.kubeclient)
	if err != nil {
		return nil, fmt.Errorf("could not extract the kubernetes runtime args: %w", err)
	}

	result, err := s.HandleCNIRequest(cmdType, k8sArgs, cniCmdArgs, s.exec, s.kubeclient)
	if err != nil {
		// Prefix error with request information for easier debugging
		return nil, fmt.Errorf("%+v %v", cniCmdArgs, err)
	}
	return result, nil
}

func (s *Server) handleDelegateRequest(r *http.Request) ([]byte, error) {
	var cr api.Request
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, err
	}
	cmdType, cniCmdArgs, err := extractCniData(&cr, s.serverConfig)
	if err != nil {
		return nil, fmt.Errorf("could not extract the CNI command args: %w", err)
	}

	k8sArgs, err := kubernetesRuntimeArgs(cr.Env, s.kubeclient)
	if err != nil {
		return nil, fmt.Errorf("could not extract the kubernetes runtime args: %w", err)
	}

	result, err := s.HandleDelegateRequest(cmdType, k8sArgs, cniCmdArgs, s.exec, s.kubeclient, cr.InterfaceAttributes)
	if err != nil {
		// Prefix error with request information for easier debugging
		return nil, fmt.Errorf("%s %v", printCmdArgs(cniCmdArgs), err)
	}
	return result, nil
}

func extractCniData(cniRequest *api.Request, overrideConf []byte) (string, *skel.CmdArgs, error) {
	cmd, ok := cniRequest.Env["CNI_COMMAND"]
	if !ok {
		return "", nil, fmt.Errorf("unexpected or missing CNI_COMMAND")
	}

	cniCmdArgs := &skel.CmdArgs{}
	cniCmdArgs.ContainerID, ok = cniRequest.Env["CNI_CONTAINERID"]
	if !ok {
		return "", nil, fmt.Errorf("missing CNI_CONTAINERID")
	}
	cniCmdArgs.Netns, ok = cniRequest.Env["CNI_NETNS"]
	if !ok {
		return "", nil, fmt.Errorf("missing CNI_NETNS")
	}

	cniCmdArgs.IfName, ok = cniRequest.Env["CNI_IFNAME"]
	if !ok {
		cniCmdArgs.IfName = "eth0"
	}

	cniArgs, found := cniRequest.Env["CNI_ARGS"]
	if !found {
		return "", nil, fmt.Errorf("missing CNI_ARGS")
	}
	cniCmdArgs.Args = cniArgs

	if overrideConf != nil {
		// trim the close bracket from multus CNI config and put the server config
		// to override CNI config with server config.
		// note: if there are two or more value in same key, then the
		// latest one is used at golang json implementation
		idx := bytes.LastIndex(cniRequest.Config, []byte("}"))
		if idx == -1 {
			return "", nil, fmt.Errorf("invalid CNI config")
		}
		cniCmdArgs.StdinData = append(cniRequest.Config[:idx], overrideConf...)
	} else {
		cniCmdArgs.StdinData = cniRequest.Config
	}

	return cmd, cniCmdArgs, nil
}

func kubernetesRuntimeArgs(cniRequestEnvVariables map[string]string, kubeClient *k8s.ClientInfo) (*types.K8sArgs, error) {
	cniEnv, err := gatherCNIArgs(cniRequestEnvVariables)
	if err != nil {
		return nil, err
	}
	podNamespace, found := cniEnv["K8S_POD_NAMESPACE"]
	if !found {
		return nil, fmt.Errorf("missing K8S_POD_NAMESPACE")
	}

	podName, found := cniEnv["K8S_POD_NAME"]
	if !found {
		return nil, fmt.Errorf("missing K8S_POD_NAME")
	}

	uid, err := podUID(kubeClient, cniEnv, podNamespace, podName)
	if err != nil {
		return nil, err
	}

	sandboxID := cniRequestEnvVariables["K8S_POD_INFRA_CONTAINER_ID"]

	return &types.K8sArgs{
		K8S_POD_NAME:               cnitypes.UnmarshallableString(podName),
		K8S_POD_NAMESPACE:          cnitypes.UnmarshallableString(podNamespace),
		K8S_POD_INFRA_CONTAINER_ID: cnitypes.UnmarshallableString(sandboxID),
		K8S_POD_UID:                cnitypes.UnmarshallableString(uid),
	}, nil
}

func gatherCNIArgs(env map[string]string) (map[string]string, error) {
	cniArgs, ok := env["CNI_ARGS"]
	if !ok {
		return nil, fmt.Errorf("missing CNI_ARGS: '%s'", env)
	}

	mapArgs := make(map[string]string)
	for _, arg := range strings.Split(cniArgs, ";") {
		parts := strings.Split(arg, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid CNI_ARG '%s'", arg)
		}
		mapArgs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return mapArgs, nil
}

func podUID(kubeclient *k8s.ClientInfo, cniArgs map[string]string, podNamespace, podName string) (string, error) {
	// UID may not be passed by all runtimes yet. Will be passed
	// by CRIO 1.20+ and containerd 1.5+ soon.
	// CRIO 1.20: https://github.com/cri-o/cri-o/pull/5029
	// CRIO 1.21: https://github.com/cri-o/cri-o/pull/5028
	// CRIO 1.22: https://github.com/cri-o/cri-o/pull/5026
	// containerd 1.6: https://github.com/containerd/containerd/pull/5640
	// containerd 1.5: https://github.com/containerd/containerd/pull/5643
	uid, found := cniArgs["K8S_POD_UID"]
	if !found {
		pod, err := kubeclient.GetPod(podNamespace, podName)
		if err != nil {
			return "", fmt.Errorf("missing pod UID; attempted to recover it from the K8s API, but failed: %w", err)
		}
		return string(pod.UID), nil
	}

	return uid, nil
}

func cmdAdd(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) ([]byte, error) {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return nil, fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdAdd for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	result, err := multus.CmdAdd(cmdArgs, exec, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("error configuring pod [%s/%s] networking: %v", namespace, podName, err)
	}
	return serializeResult(result)
}

func cmdDel(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdDel for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	return multus.CmdDel(cmdArgs, exec, kubeClient)
}

func cmdCheck(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdCheck for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	return multus.CmdCheck(cmdArgs, exec, kubeClient)
}

func serializeResult(result cnitypes.Result) ([]byte, error) {
	// cni result is converted to latest here and decoded to specific cni version at multus-shim
	realResult, err := cni100.NewResultFromResult(result)
	if err != nil {
		return nil, fmt.Errorf("failed to generate the CNI result: %w", err)
	}

	responseBytes, err := json.Marshal(&api.Response{Result: realResult})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pod request response: %v", err)
	}
	return responseBytes, nil
}

func cmdDelegateAdd(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo, multusConfig *types.NetConf, interfaceAttributes *api.DelegateInterfaceAttributes) ([]byte, error) {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return nil, fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}
	pod, err := multus.GetPod(kubeClient, k8sArgs, false)
	if err != nil {
		return nil, err
	}

	// copy deleate annotation into network selection element
	var selectionElement *types.NetworkSelectionElement
	if interfaceAttributes != nil {
		selectionElement = &types.NetworkSelectionElement{}
		if interfaceAttributes.MacRequest != "" {
			selectionElement.MacRequest = interfaceAttributes.MacRequest
		}
		if interfaceAttributes.IPRequest != nil {
			selectionElement.IPRequest = interfaceAttributes.IPRequest
		}
		if interfaceAttributes.CNIArgs != nil {
			selectionElement.CNIArgs = interfaceAttributes.CNIArgs
		}
	}

	delegateCNIConf, err := types.LoadDelegateNetConf(cmdArgs.StdinData, selectionElement, "", "")
	if err != nil {
		return nil, err
	}

	logging.Debugf("CmdDelegateAdd for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	rt, _ := types.CreateCNIRuntimeConf(cmdArgs, k8sArgs, cmdArgs.IfName, nil, delegateCNIConf)
	result, err := multus.DelegateAdd(exec, kubeClient, pod, delegateCNIConf, rt, multusConfig)
	if err != nil {
		return nil, fmt.Errorf("error configuring pod [%s/%s] networking: %v", namespace, podName, err)
	}

	return serializeResult(result)
}

func cmdDelegateCheck(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, _ *k8s.ClientInfo, multusConfig *types.NetConf) error {
	delegateCNIConf := &types.DelegateNetConf{}
	if err := json.Unmarshal(cmdArgs.StdinData, delegateCNIConf); err != nil {
		return err
	}
	delegateCNIConf.Bytes = cmdArgs.StdinData
	rt, _ := types.CreateCNIRuntimeConf(cmdArgs, k8sArgs, cmdArgs.IfName, nil, delegateCNIConf)
	return multus.DelegateCheck(exec, delegateCNIConf, rt, multusConfig)
}

// note: this function may send back error to the client. In cni spec, command DEL should NOT send any error
// because container deletion follows cni DEL command. But in delegateDel case, container is not removed by
// this delegateDel, hence we decide to send error message to the request sender.
func cmdDelegateDel(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo, multusConfig *types.NetConf) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}
	pod, err := multus.GetPod(kubeClient, k8sArgs, false)
	if err != nil {
		return err
	}

	delegateCNIConf, err := types.LoadDelegateNetConf(cmdArgs.StdinData, nil, "", "")
	if err != nil {
		return err
	}
	rt, _ := types.CreateCNIRuntimeConf(cmdArgs, k8sArgs, cmdArgs.IfName, nil, delegateCNIConf)
	return multus.DelegateDel(exec, pod, delegateCNIConf, rt, multusConfig)
}

// LoadDaemonNetConf loads the configuration for the multus daemon
func LoadDaemonNetConf(config []byte) (*ControllerNetConf, error) {
	daemonNetConf := &ControllerNetConf{
		SocketDir: DefaultMultusRunDir,
	}
	if err := json.Unmarshal(config, daemonNetConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshall the daemon configuration: %w", err)
	}

	logging.SetLogStderr(daemonNetConf.LogToStderr)
	if daemonNetConf.LogFile != DefaultMultusDaemonConfigFile {
		logging.SetLogFile(daemonNetConf.LogFile)
	}
	if daemonNetConf.LogLevel != "" {
		logging.SetLogLevel(daemonNetConf.LogLevel)
	}
	daemonNetConf.ConfigFileContents = config

	return daemonNetConf, nil
}
