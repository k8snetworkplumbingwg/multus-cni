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
//

package cni

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/current"
	"github.com/gorilla/mux"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/config"
	k8s "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/multus"
)

// HandleCNIRequest is the CNI server handler function; it is invoked whenever
// a CNI request is processed.
func HandleCNIRequest(request *PodRequest) ([]byte, error) {
	var result []byte
	var err error

	logging.Verbosef("%+v %s starting CNI request %+v", request, request.Command, request)
	switch request.Command {
	case "ADD":
		result, err = request.cmdAdd()
	case "DEL":
		err = request.cmdDelete()
	case "CHECK":
		err = request.cmdCheck()
	default:
	}
	logging.Verbosef("%s finished CNI request %+v, result: %q, err: %v", request.Command, request, string(result), err)
	if err != nil {
		// Prefix errors with request info for easier failure debugging
		return nil, fmt.Errorf("%+v ERRORED: %v", request, err)
	}
	return result, nil
}

// ServerListener creates a listener to a unix socket located in `socketPath`
func ServerListener(socketPath string) (net.Listener, error) {
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
func NewCNIServer(rundir string) (*Server, error) {
	kubeClient, err := k8s.InClusterK8sClient()
	if err != nil {
		return nil, fmt.Errorf("error getting k8s client: %v", err)
	}

	return newCNIServer(rundir, kubeClient, nil)
}

func newCNIServer(rundir string, kubeClient *k8s.ClientInfo, exec invoke.Exec) (*Server, error) {
	router := mux.NewRouter()
	s := &Server{
		Server: http.Server{
			Handler: router,
		},
		rundir:      rundir,
		requestFunc: HandleCNIRequest,
		kubeclient:  kubeClient,
		exec:        exec,
	}

	router.NotFoundHandler = http.HandlerFunc(http.NotFound)
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		result, err := s.handleCNIRequest(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
			return
		}

		// Empty response JSON means success with no body
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(result); err != nil {
			_ = logging.Errorf("Error writing HTTP response: %v", err)
		}
	}).Methods("POST")

	return s, nil
}

func (s *Server) handleCNIRequest(r *http.Request) ([]byte, error) {
	var cr Request
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, err
	}
	req, err := cniRequestToPodRequest(&cr, s.kubeclient, s.exec)
	if err != nil {
		return nil, err
	}

	result, err := s.requestFunc(req)
	if err != nil {
		// Prefix error with request information for easier debugging
		return nil, fmt.Errorf("%+v %v", req, err)
	}
	return result, nil
}

func cniRequestToPodRequest(cniRequest *Request, kubeclient *k8s.ClientInfo, exec invoke.Exec) (*PodRequest, error) {
	cmd, ok := cniRequest.Env["CNI_COMMAND"]
	if !ok {
		return nil, fmt.Errorf("unexpected or missing CNI_COMMAND")
	}

	req := &PodRequest{
		Command:    command(cmd),
		exec:       exec,
		kubeClient: kubeclient,
	}

	req.ContainerID, ok = cniRequest.Env["CNI_CONTAINERID"]
	if !ok {
		return nil, fmt.Errorf("missing CNI_CONTAINERID")
	}
	req.Netns, ok = cniRequest.Env["CNI_NETNS"]
	if !ok {
		return nil, fmt.Errorf("missing CNI_NETNS")
	}

	req.IfName, ok = cniRequest.Env["CNI_IFNAME"]
	if !ok {
		req.IfName = "eth0"
	}

	cniArgs, found := cniRequest.Env["CNI_ARGS"]
	if !found {
		return nil, fmt.Errorf("missing CNI_ARGS")
	}
	req.Args = cniArgs
	req.StdinData = cniRequest.Config

	cniEnv, err := gatherCNIArgs(cniRequest.Env)
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

	uid, err := podUID(kubeclient, cniEnv, podNamespace, podName)
	if err != nil {
		return nil, err
	}

	sandboxID := cniRequest.Env["K8S_POD_INFRA_CONTAINER_ID"]

	req.K8S_POD_NAMESPACE = cnitypes.UnmarshallableString(podNamespace)
	req.K8S_POD_NAME = cnitypes.UnmarshallableString(podName)
	req.K8S_POD_INFRA_CONTAINER_ID = cnitypes.UnmarshallableString(sandboxID)
	req.K8S_POD_UID = cnitypes.UnmarshallableString(uid)

	return req, nil
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

func (pr *PodRequest) cniSkel() *skel.CmdArgs {
	return &skel.CmdArgs{
		ContainerID: pr.ContainerID,
		Netns:       pr.Netns,
		IfName:      pr.IfName,
		Args:        pr.Args,
		Path:        pr.Path,
		StdinData:   pr.StdinData,
	}
}

func (pr *PodRequest) cmdAdd() ([]byte, error) {
	namespace := string(pr.K8S_POD_NAMESPACE)
	podName := string(pr.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return nil, fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdAdd for [%s/%s]. CNI conf: %+v", namespace, podName, *pr)
	result, err := multus.CmdAdd(pr.cniSkel(), pr.exec, pr.kubeClient)
	if err != nil {
		return nil, fmt.Errorf("error configuring pod [%s/%s] networking: %v", namespace, podName, err)
	}
	return serializeResult(result)
}

func (pr *PodRequest) cmdDelete() error {
	namespace := string(pr.K8S_POD_NAMESPACE)
	podName := string(pr.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdDel for [%s/%s]. CNI conf: %+v", namespace, podName, *pr)
	return multus.CmdDel(pr.cniSkel(), pr.exec, pr.kubeClient)
}

func (pr *PodRequest) cmdCheck() error {
	namespace := string(pr.K8S_POD_NAMESPACE)
	podName := string(pr.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdCheck for [%s/%s]. CNI conf: %+v", namespace, podName, *pr)
	return multus.CmdCheck(pr.cniSkel(), pr.exec, pr.kubeClient)
}

func serializeResult(result cnitypes.Result) ([]byte, error) {
	realResult, err := cnicurrent.NewResultFromResult(result)
	if err != nil {
		return nil, fmt.Errorf("failed to generate the CNI result: %w", err)
	}

	responseBytes, err := json.Marshal(&Response{Result: realResult})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pod request response: %v", err)
	}
	return responseBytes, nil
}
