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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

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

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netdefclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	netdefinformer "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions"
	netdefinformerv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions/k8s.cni.cncf.io/v1"

	kapi "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
	informerfactory "k8s.io/client-go/informers"
	v1coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
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
func (s *Server) HandleCNIRequest(cmd string, k8sArgs *types.K8sArgs, cniCmdArgs *skel.CmdArgs) ([]byte, error) {
	var result []byte
	var err error

	logging.Verbosef("%s starting CNI request %s", cmd, printCmdArgs(cniCmdArgs))
	switch cmd {
	case "ADD":
		result, err = s.cmdAdd(cniCmdArgs, k8sArgs)
	case "DEL":
		err = s.cmdDel(cniCmdArgs, k8sArgs)
	case "CHECK":
		err = s.cmdCheck(cniCmdArgs, k8sArgs)
	default:
		return []byte(""), fmt.Errorf("unknown cmd type: %s", cmd)
	}
	logging.Verbosef("%s finished CNI request %s, result: %q, err: %v", cmd, printCmdArgs(cniCmdArgs), string(result), err)
	return result, err
}

// HandleDelegateRequest is the CNI server handler function; it is invoked whenever
// a CNI request is processed as delegate CNI request.
func (s *Server) HandleDelegateRequest(cmd string, k8sArgs *types.K8sArgs, cniCmdArgs *skel.CmdArgs, interfaceAttributes *api.DelegateInterfaceAttributes) ([]byte, error) {
	var result []byte
	var err error

	multusConfig := types.GetDefaultNetConf()
	if err = json.Unmarshal(s.serverConfig, multusConfig); err != nil {
		return nil, err
	}

	logging.Verbosef("%s starting delegate request %s", cmd, printCmdArgs(cniCmdArgs))
	switch cmd {
	case "ADD":
		result, err = s.cmdDelegateAdd(cniCmdArgs, k8sArgs, multusConfig, interfaceAttributes)
	case "DEL":
		err = s.cmdDelegateDel(cniCmdArgs, k8sArgs, multusConfig)
	case "CHECK":
		err = s.cmdDelegateCheck(cniCmdArgs, k8sArgs, multusConfig)
	default:
		return []byte(""), fmt.Errorf("unknown cmd type: %s", cmd)
	}
	logging.Verbosef("%s finished Delegate request %s, result: %q, err: %v", cmd, printCmdArgs(cniCmdArgs), string(result), err)
	return result, err
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

// Informer transform to trim object fields for memory efficiency.
func informerObjectTrim(obj interface{}) (interface{}, error) {
	if accessor, err := meta.Accessor(obj); err == nil {
		accessor.SetManagedFields(nil)
	}
	if pod, ok := obj.(*kapi.Pod); ok {
		pod.Spec.Volumes = []kapi.Volume{}
		for i := range pod.Spec.Containers {
			pod.Spec.Containers[i].Command = nil
			pod.Spec.Containers[i].Args = nil
			pod.Spec.Containers[i].Env = nil
			pod.Spec.Containers[i].VolumeMounts = nil
		}
	}
	return obj, nil
}

func newNetDefInformer(netWatchClient netdefclient.Interface) (netdefinformer.SharedInformerFactory, cache.SharedIndexInformer) {
	const resyncInterval time.Duration = 1 * time.Second

	informerFactory := netdefinformer.NewSharedInformerFactoryWithOptions(netWatchClient, resyncInterval)
	netdefInformer := informerFactory.InformerFor(&netdefv1.NetworkAttachmentDefinition{}, func(client netdefclient.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		return netdefinformerv1.NewNetworkAttachmentDefinitionInformer(
			client,
			kapi.NamespaceAll,
			resyncPeriod,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	})

	return informerFactory, netdefInformer
}

func newPodInformer(watchClient kubernetes.Interface, nodeName string) (internalinterfaces.SharedInformerFactory, cache.SharedIndexInformer) {
	var tweakFunc internalinterfaces.TweakListOptionsFunc
	if nodeName != "" {
		logging.Verbosef("Filtering pod watch for node %q", nodeName)
		// Only watch for local pods
		tweakFunc = func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
		}
	}

	const resyncInterval time.Duration = 1 * time.Second

	informerFactory := informerfactory.NewSharedInformerFactoryWithOptions(watchClient, resyncInterval, informerfactory.WithTransform(informerObjectTrim))
	podInformer := informerFactory.InformerFor(&kapi.Pod{}, func(c kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		return v1coreinformers.NewFilteredPodInformer(
			c,
			kapi.NamespaceAll,
			resyncPeriod,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
			tweakFunc)
	})
	return informerFactory, podInformer
}

func isPerNodeCertEnabled(config *PerNodeCertificate) (bool, error) {
	if config != nil && config.Enabled {
		if config.BootstrapKubeconfig != "" && config.CertDir != "" {
			return true, nil
		}
		return true, logging.Errorf("failed to configure PerNodeCertificate: enabled: %v, BootstrapKubeconfig: %q, CertDir: %q", config.Enabled, config.BootstrapKubeconfig, config.CertDir)
	}
	return false, nil
}

// NewCNIServer creates and returns a new Server object which will listen on a socket in the given path
func NewCNIServer(daemonConfig *ControllerNetConf, serverConfig []byte, ignoreReadinessIndicator bool) (*Server, error) {
	var kubeClient *k8s.ClientInfo
	enabled, err := isPerNodeCertEnabled(daemonConfig.PerNodeCertificate)
	if enabled {
		if err != nil {
			return nil, err
		}
		perNodeCertConfig := daemonConfig.PerNodeCertificate
		nodeName := os.Getenv("MULTUS_NODE_NAME")
		if nodeName == "" {
			return nil, logging.Errorf("error getting node name for perNodeCertificate, please check manifest to have MULTUS_NODE_NAME")
		}

		certDuration := DefaultCertDuration
		if perNodeCertConfig.CertDuration != "" {
			certDuration, err = time.ParseDuration(perNodeCertConfig.CertDuration)
			if err != nil {
				return nil, logging.Errorf("failed to parse certDuration: %v", err)
			}
		}

		kubeClient, err = k8s.PerNodeK8sClient(nodeName, perNodeCertConfig.BootstrapKubeconfig, certDuration, perNodeCertConfig.CertDir)
		if err != nil {
			return nil, logging.Errorf("error getting perNodeClient: %v", err)
		}
	} else {
		kubeClient, err = k8s.InClusterK8sClient()
		if err != nil {
			return nil, fmt.Errorf("error getting k8s client: %v", err)
		}
	}

	exec := invoke.Exec(nil)
	if daemonConfig.ChrootDir != "" {
		chrootExec := &ChrootExec{
			Stderr:    os.Stderr,
			chrootDir: daemonConfig.ChrootDir,
		}
		exec = chrootExec
		logging.Verbosef("server configured with chroot: %s", daemonConfig.ChrootDir)
	}

	return newCNIServer(daemonConfig.SocketDir, kubeClient, exec, serverConfig, ignoreReadinessIndicator)
}

func newCNIServer(rundir string, kubeClient *k8s.ClientInfo, exec invoke.Exec, servConfig []byte, ignoreReadinessIndicator bool) (*Server, error) {
	podInformerFactory, podInformer := newPodInformer(kubeClient.WatchClient, os.Getenv("MULTUS_NODE_NAME"))
	netdefInformerFactory, netdefInformer := newNetDefInformer(kubeClient.NetWatchClient)
	kubeClient.SetK8sClientInformers(podInformer, netdefInformer)

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
		podInformerFactory:       podInformerFactory,
		podInformer:              podInformer,
		netdefInformerFactory:    netdefInformerFactory,
		netdefInformer:           netdefInformer,
		ignoreReadinessIndicator: ignoreReadinessIndicator,
	}
	s.SetKeepAlivesEnabled(false)

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

// Start starts the server and begins serving on the given listener
func (s *Server) Start(ctx context.Context, l net.Listener) {
	s.podInformerFactory.Start(ctx.Done())
	s.netdefInformerFactory.Start(ctx.Done())

	// Give the initial sync some time to complete in large clusters, but
	// don't wait forever
	waitCtx, waitCancel := context.WithTimeout(ctx, 20*time.Second)
	if !cache.WaitForCacheSync(waitCtx.Done(), s.podInformer.HasSynced) {
		logging.Errorf("failed to sync pod informer cache")
	}
	waitCancel()

	// Give the initial sync some time to complete in large clusters, but
	// don't wait forever
	waitCtx, waitCancel = context.WithTimeout(ctx, 20*time.Second)
	if !cache.WaitForCacheSync(waitCtx.Done(), s.netdefInformer.HasSynced) {
		logging.Errorf("failed to sync net-attach-def informer cache")
	}
	waitCancel()

	go func() {
		utilwait.UntilWithContext(ctx, func(_ context.Context) {
			logging.Debugf("open for business")
			if err := s.Serve(l); err != nil {
				utilruntime.HandleError(fmt.Errorf("CNI server Serve() failed: %v", err))
			}
		}, 0)
	}()
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
	cmdType, cniCmdArgs, err := s.extractCniData(&cr, s.serverConfig)
	if err != nil {
		return nil, fmt.Errorf("could not extract the CNI command args: %w", err)
	}

	k8sArgs, err := kubernetesRuntimeArgs(cr.Env, s.kubeclient)
	if err != nil {
		return nil, fmt.Errorf("could not extract the kubernetes runtime args: %w", err)
	}

	result, err := s.HandleCNIRequest(cmdType, k8sArgs, cniCmdArgs)
	if err != nil {
		// Prefix error with request information for easier debugging
		return nil, fmt.Errorf("%s ERRORED: %v", printCmdArgs(cniCmdArgs), err)
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
	cmdType, cniCmdArgs, err := s.extractCniData(&cr, s.serverConfig)
	if err != nil {
		return nil, fmt.Errorf("could not extract the CNI command args: %w", err)
	}

	k8sArgs, err := kubernetesRuntimeArgs(cr.Env, s.kubeclient)
	if err != nil {
		return nil, fmt.Errorf("could not extract the kubernetes runtime args: %w", err)
	}

	result, err := s.HandleDelegateRequest(cmdType, k8sArgs, cniCmdArgs, cr.InterfaceAttributes)
	if err != nil {
		// Prefix error with request information for easier debugging
		return nil, fmt.Errorf("%s ERRORED: %v", printCmdArgs(cniCmdArgs), err)
	}
	return result, nil
}

func overrideCNIConfigWithServerConfig(cniConf []byte, overrideConf []byte, ignoreReadinessIndicator bool) ([]byte, error) {
	if len(overrideConf) == 0 {
		return cniConf, nil
	}

	var cni map[string]interface{}
	if err := json.Unmarshal(cniConf, &cni); err != nil {
		return nil, fmt.Errorf("failed to unmarshall CNI config: %w", err)
	}

	var override map[string]interface{}
	if err := json.Unmarshal(overrideConf, &override); err != nil {
		return nil, fmt.Errorf("failed to unmarshall CNI override config: %w", err)
	}

	// Copy each key of the override config into the CNI config except for
	// a few specific keys
	ignoreKeys := sets.NewString()
	if ignoreReadinessIndicator {
		ignoreKeys.Insert("readinessindicatorfile")
	}
	for overrideKey, overrideVal := range override {
		if !ignoreKeys.Has(overrideKey) {
			cni[overrideKey] = overrideVal
		}
	}

	newBytes, err := json.Marshal(cni)
	if err != nil {
		return nil, fmt.Errorf("failed ot marshall new CNI config with overrides: %w", err)
	}

	return newBytes, nil
}

func (s *Server) extractCniData(cniRequest *api.Request, overrideConf []byte) (string, *skel.CmdArgs, error) {
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

	var err error
	cniCmdArgs.StdinData, err = overrideCNIConfigWithServerConfig(cniRequest.Config, overrideConf, s.ignoreReadinessIndicator)
	if err != nil {
		return "", nil, err
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

func (s *Server) cmdAdd(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs) ([]byte, error) {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return nil, fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdAdd for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	result, err := multus.CmdAdd(cmdArgs, s.exec, s.kubeclient)
	if err != nil {
		return nil, fmt.Errorf("error configuring pod [%s/%s] networking: %v", namespace, podName, err)
	}
	return serializeResult(result)
}

func (s *Server) cmdDel(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdDel for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	return multus.CmdDel(cmdArgs, s.exec, s.kubeclient)
}

func (s *Server) cmdCheck(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}

	logging.Debugf("CmdCheck for [%s/%s]. CNI conf: %+v", namespace, podName, *cmdArgs)
	return multus.CmdCheck(cmdArgs, s.exec, s.kubeclient)
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

func (s *Server) cmdDelegateAdd(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, multusConfig *types.NetConf, interfaceAttributes *api.DelegateInterfaceAttributes) ([]byte, error) {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return nil, fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}
	pod, err := multus.GetPod(s.kubeclient, k8sArgs, false)
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
	result, err := multus.DelegateAdd(s.exec, s.kubeclient, pod, delegateCNIConf, rt, multusConfig)
	if err != nil {
		return nil, fmt.Errorf("error configuring pod [%s/%s] networking: %v", namespace, podName, err)
	}

	return serializeResult(result)
}

func (s *Server) cmdDelegateCheck(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, multusConfig *types.NetConf) error {
	delegateCNIConf := &types.DelegateNetConf{}
	if err := json.Unmarshal(cmdArgs.StdinData, delegateCNIConf); err != nil {
		return err
	}
	delegateCNIConf.Bytes = cmdArgs.StdinData
	rt, _ := types.CreateCNIRuntimeConf(cmdArgs, k8sArgs, cmdArgs.IfName, nil, delegateCNIConf)
	return multus.DelegateCheck(s.exec, delegateCNIConf, rt, multusConfig)
}

// note: this function may send back error to the client. In cni spec, command DEL should NOT send any error
// because container deletion follows cni DEL command. But in delegateDel case, container is not removed by
// this delegateDel, hence we decide to send error message to the request sender.
func (s *Server) cmdDelegateDel(cmdArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, multusConfig *types.NetConf) error {
	namespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	if namespace == "" || podName == "" {
		return fmt.Errorf("required CNI variable missing. pod name: %s; pod namespace: %s", podName, namespace)
	}
	pod, err := multus.GetPod(s.kubeclient, k8sArgs, false)
	if err != nil {
		return err
	}

	delegateCNIConf, err := types.LoadDelegateNetConf(cmdArgs.StdinData, nil, "", "")
	if err != nil {
		return err
	}
	rt, _ := types.CreateCNIRuntimeConf(cmdArgs, k8sArgs, cmdArgs.IfName, nil, delegateCNIConf)
	return multus.DelegateDel(s.exec, pod, delegateCNIConf, rt, multusConfig)
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
