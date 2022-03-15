package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/skel"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/cniclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

const allNamespaces = ""
const controllerName = "multus-cni-pod-networks-controller"
const podNetworksAnnot = "k8s.v1.cni.cncf.io/networks"
const podNetworkStatus = "k8s.v1.cni.cncf.io/network-status"

// PodNetworksController handles the cncf networks annotations update, and
// triggers adding / removing networks from a running pod.
type PodNetworksController struct {
	k8sClientSet     kubernetes.Interface
	podsSynced       cache.InformerSynced
	workqueue        workqueue.RateLimitingInterface
	recorder         record.EventRecorder
	containerRuntime containerruntimes.ContainerRuntime
	cniPlugin        *cniclient.CniPlugin
	confDir          string
	k8sClientConfig  *rest.Config
}

// NewPodNetworksController returns new PodNetworksController instance
func NewPodNetworksController(
	k8sClientSet kubernetes.Interface,
	podInformer cache.SharedIndexInformer,
	cniPlugin *cniclient.CniPlugin,
	confDir string,
	k8sClientConfig *rest.Config) (*PodNetworksController, error) {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logging.Verbosef)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: k8sClientSet.CoreV1().Events(allNamespaces)})

	containerRuntime, err := containerruntimes.NewRuntime("/run/containerd/containerd.sock", containerruntimes.Containerd)
	if err != nil {
		return nil, err
	}

	podNetworksController := &PodNetworksController{
		k8sClientSet: k8sClientSet,
		podsSynced:   podInformer.HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"pod-networks-updates"),
		recorder:         eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName}),
		containerRuntime: *containerRuntime,
		cniPlugin:        cniPlugin,
		confDir:          confDir,
		k8sClientConfig:  k8sClientConfig,
	}

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: podNetworksController.handlePodUpdate,
	})

	return podNetworksController, nil
}

// Start runs worker thread after performing cache synchronization
func (pnc *PodNetworksController) Start(stopChan <-chan struct{}) {
	logging.Verbosef("starting network controller")
	defer pnc.workqueue.ShutDown()

	if ok := cache.WaitForCacheSync(stopChan, pnc.podsSynced); !ok {
		logging.Verbosef("failed waiting for caches to sync")
	}

	go wait.Until(pnc.worker, time.Second, stopChan)
	<-stopChan
	logging.Verbosef("shutting down network controller")
	return
}

func (pnc *PodNetworksController) worker() {
	for pnc.processNextWorkItem() {
	}
}

func (pnc *PodNetworksController) processNextWorkItem() bool {
	key, shouldQuit := pnc.workqueue.Get()
	if shouldQuit {
		return false
	}
	defer pnc.workqueue.Done(key)

	return true
}

func (pnc *PodNetworksController) handlePodUpdate(oldObj interface{}, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	if reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
		return
	}
	podNamespace := oldPod.GetNamespace()
	podName := oldPod.GetName()
	logging.Debugf("pod [%s] update", fmt.Sprintf("%s/%s", podNamespace, podName))

	oldNetworkSelectionElements, err := networkSelectionElements(oldPod.Annotations, podNamespace)
	if err != nil {
		_ = logging.Errorf("failed to compute the network selection elements from the *old* pod")
		return
	}

	newNetworkSelectionElements, err := networkSelectionElements(newPod.Annotations, podNamespace)
	if err != nil {
		_ = logging.Errorf("failed to compute the network selection elements from the *new* pod")
		return
	}

	toAdd := exclusiveNetworks(newNetworkSelectionElements, oldNetworkSelectionElements)
	if err := pnc.addNetworks(toAdd, newPod); err != nil {
		_ = logging.Errorf("failed to *add* networks: %v", err)
	}

	toRemove := exclusiveNetworks(oldNetworkSelectionElements, newNetworkSelectionElements)
	if err := pnc.removeNetworks(toRemove, newPod); err != nil {
		_ = logging.Errorf("failed to *remove* networks: %v", err)
	}
}

func (pnc *PodNetworksController) addNetworks(netsToAdd []types.NetworkSelectionElement, pod *corev1.Pod) error {
	for _, netToAdd := range netsToAdd {
		cniParams, err := pnc.getCNIParams(pod, netToAdd)
		if err != nil {
			return logging.Errorf("failed to extract CNI params to hotplug new interface for pod %s: %v", pod.GetName(), err)
		}
		logging.Verbosef("CNI params for pod %s: %+v", pod.GetName(), cniParams)

		k8sClient, err := pnc.k8sClient()
		if err != nil {
			return err
		}

		delegateConf, _, err := k8sclient.GetKubernetesDelegate(k8sClient, &netToAdd, pnc.confDir, pod, nil)
		if err != nil {
			return fmt.Errorf("error retrieving the delegate info: %w", err)
		}

		k8sArgs, _ := k8sclient.GetK8sArgs(&cniParams.CniCmdArgs)
		multusNetconf, err := pnc.multusConf()
		if err != nil {
			return logging.Errorf("failed to retrieve the multus network: %v", err)
		}

		logging.Verbosef("the multus config: %+v", *multusNetconf)
		if strings.HasPrefix(multusNetconf.BinDir, "/host") {
			strings.ReplaceAll(multusNetconf.BinDir, "/host", "")
		}
		logging.Verbosef("the MUTATED multus config: %+v", *multusNetconf)
		result, err := pnc.cniPlugin.AddNetworks(k8sClient, pod, &cniParams.CniCmdArgs, k8sArgs, delegateConf, cniParams.BuildRuntimeConf(), multusNetconf)
		if err != nil {
			return logging.Errorf("failed to remove network. error: %v", err)
		}

		logging.Verbosef("added network %s with iface name %s to pod %s; res: %+v", netToAdd.Name, cniParams.CniCmdArgs.IfName, pod.GetName(), result)
	}
	return nil
}

func (pnc *PodNetworksController) removeNetworks(netsToRemove []types.NetworkSelectionElement, pod *corev1.Pod) error {
	var (
		multusNetconf *types.NetConf
		podNetStatus  []nadv1.NetworkStatus
	)
	k8sClient, err := pnc.k8sClient()
	if err != nil {
		return err
	}

	for _, netToRemove := range netsToRemove {
		cniParams, err := pnc.getCNIParams(pod, netToRemove)
		if err != nil {
			return logging.Errorf("failed to extract CNI params to remove existing interface from pod %s: %v", pod.GetName(), err)
		}
		logging.Verbosef("CNI params for pod %s: %+v", pod.GetName(), cniParams)

		delegateConf, _, err := k8sclient.GetKubernetesDelegate(k8sClient, &netToRemove, pnc.confDir, pod, nil)
		if err != nil {
			return fmt.Errorf("error retrieving the delegate info: %w", err)
		}
		k8sArgs, _ := k8sclient.GetK8sArgs(&cniParams.CniCmdArgs)

		multusNetconf, err = pnc.multusConf()
		if err != nil {
			return logging.Errorf("failed to retrieve the multus network: %v", err)
		}

		rtConf := types.DelegateRuntimeConfig(cniParams.CniCmdArgs.ContainerID, delegateConf, multusNetconf.RuntimeConfig, cniParams.CniCmdArgs.IfName)
		logging.Verbosef("the multus config: %+v", *multusNetconf)
		if strings.HasPrefix(multusNetconf.BinDir, "/host") {
			strings.ReplaceAll(multusNetconf.BinDir, "/host", "")
		}
		logging.Verbosef("the MUTATED multus config: %+v", *multusNetconf)
		if err := pnc.cniPlugin.RemoveNetworks(pod, &cniParams.CniCmdArgs, k8sArgs, delegateConf, rtConf, multusNetconf); err != nil {
			//if err := multus.DelPlugins(nil, pod, &cniParams.CniCmdArgs, k8sArgs, []*types.DelegateNetConf{delegateConf}, 0, rtConf, multusNetconf); err != nil {
			return logging.Errorf("failed to remove network. error: %v", err)
		}
		logging.Verbosef("removed network %s from pod %s with interface name: %s", netToRemove.Name, pod.GetName(), cniParams.CniCmdArgs.IfName)

		oldNetStatus, err := networkStatus(pod.Annotations)
		if err != nil {
			return fmt.Errorf("failed to extract the pod's network status: %+v", err)
		}

		for _, netStatus := range oldNetStatus {
			if fmt.Sprintf("%s/%s", netToRemove.Namespace, netToRemove.Name) != netStatus.Name {
				podNetStatus = append(podNetStatus, netStatus)
			}
		}
	}

	if multusNetconf == nil {
		multusNetconf, err = pnc.multusConf()
		if err != nil {
			logging.Verbosef("error accessing the multus configuration: %+v", err)
		}
	}

	if err := k8sclient.SetPodNetworkStatusAnnotation(k8sClient, pod.GetName(), pod.GetNamespace(), string(pod.GetUID()), podNetStatus, multusNetconf); err != nil {
		// error happen but continue to delete
		logging.Errorf("Multus: error unsetting the networks status: %v", err)
	}

	return nil
}

func (pnc *PodNetworksController) k8sClient() (*k8sclient.ClientInfo, error) {
	multusAPIClient, err := netclient.NewForConfig(pnc.k8sClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate a multus API client from the in-pod cluster config: %w", err)
	}
	k8sClient := &k8sclient.ClientInfo{
		Client:           pnc.k8sClientSet,
		NetClient:        multusAPIClient,
		EventBroadcaster: nil,
		EventRecorder:    pnc.recorder,
	}
	return k8sClient, nil
}

func networkSelectionElements(podAnnotations map[string]string, podNamespace string) ([]*types.NetworkSelectionElement, error) {
	podNetworks, ok := podAnnotations[podNetworksAnnot]
	if !ok {
		return nil, fmt.Errorf("the pod is missing the \"%s\" annotation on its annotations: %+v", podNetworksAnnot, podAnnotations)
	}
	podNetworkSelectionElements, err := types.ParsePodNetworkAnnotation(podNetworks, podNamespace)
	if err != nil {
		// already logged inside `ParsePodNetworkAnnotation` func
		return nil, err
	}
	return podNetworkSelectionElements, nil
}

func networkStatus(podAnnotations map[string]string) ([]nadv1.NetworkStatus, error) {
	podNetworkstatus, ok := podAnnotations[nadv1.NetworkStatusAnnot]
	if !ok {
		return nil, fmt.Errorf("the pod is missing the \"%s\" annotation on its annotations: %+v", podNetworksAnnot, podAnnotations)
	}
	var netStatus []nadv1.NetworkStatus
	if err := json.Unmarshal([]byte(podNetworkstatus), &netStatus); err != nil {
		return nil, err
	}

	return netStatus, nil
}

func exclusiveNetworks(needles []*types.NetworkSelectionElement, haystack []*types.NetworkSelectionElement) []types.NetworkSelectionElement {
	setOfNeedles := indexNetworkSelectionElements(needles)
	haystackSet := indexNetworkSelectionElements(haystack)

	var unmatchedNetworks []types.NetworkSelectionElement
	for needleNetName, needle := range setOfNeedles {
		if _, ok := haystackSet[needleNetName]; !ok {
			unmatchedNetworks = append(unmatchedNetworks, needle)
		}
	}
	return unmatchedNetworks
}

func indexNetworkSelectionElements(list []*types.NetworkSelectionElement) map[string]types.NetworkSelectionElement {
	indexedNetworkSelectionElements := make(map[string]types.NetworkSelectionElement)
	for k := range list {
		indexedNetworkSelectionElements[networkSelectionElementIndexKey(*list[k])] = *list[k]
	}
	return indexedNetworkSelectionElements
}

func networkSelectionElementIndexKey(netSelectionElement types.NetworkSelectionElement) string {
	if netSelectionElement.InterfaceRequest != "" {
		return fmt.Sprintf(
			"%s/%s/%s",
			netSelectionElement.Namespace,
			netSelectionElement.Name,
			netSelectionElement.InterfaceRequest)
	}

	return fmt.Sprintf(
		"%s/%s",
		netSelectionElement.Namespace,
		netSelectionElement.Name)
}

func (pnc *PodNetworksController) getCNIParams(podObj *corev1.Pod, netSelectionElement types.NetworkSelectionElement) (*cniclient.CNIParams, error) {
	podName := podObj.ObjectMeta.Name
	namespace := podObj.ObjectMeta.Namespace
	if containerID := podContainerID(podObj); containerID != "" {
		netns, err := pnc.containerRuntime.NetNS(containerID)
		if err != nil {
			return nil, fmt.Errorf("failed to get netns for container [%s] netns: %w", containerID, err)
		}

		interfaceName := netSelectionElement.InterfaceRequest
		if interfaceName == "" {
			// TODO: what is the correct default ?... for hotplug, at least,
			// this *must* be defined.
			interfaceName = "net1"
		}

		cmdArgs := skel.CmdArgs{
			ContainerID: containerID,
			Netns:       netns,
			IfName:      interfaceName,
			Args:        fmt.Sprintf("IgnoreUnknown=true;K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", podName, namespace),
			Path:        "/opt/cni/bin", // TODO: what to put here ?
			StdinData:   nil,            // TODO: what to put here ?
		}
		return &cniclient.CNIParams{
			Namespace:   namespace,
			PodName:     podName,
			NetworkName: netSelectionElement.Name,
			CniCmdArgs:  cmdArgs,
		}, nil
	}
	return nil, fmt.Errorf("failed to get pod %s container ID", podName)
}

func podContainerID(pod *corev1.Pod) string {
	cidURI := pod.Status.ContainerStatuses[0].ContainerID
	// format is docker://<cid>
	parts := strings.Split(cidURI, "//")
	if len(parts) > 1 {
		return parts[1]
	}
	return cidURI
}

func (pnc *PodNetworksController) multusConf() (*types.NetConf, error) {
	multusConfPath := types.CniPluginConfigFilePath(pnc.confDir, types.MultusConfigFileName)
	multusConfData, err := ioutil.ReadFile(multusConfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the multus conf from %s: %w", multusConfPath, err)
	}

	return types.LoadNetConf(multusConfData)
}
