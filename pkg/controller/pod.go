package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"reflect"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/skel"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	v1coreinformerfactory "k8s.io/client-go/informers"
	v1corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	nadinformers "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions"
	nadlisterv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/listers/k8s.cni.cncf.io/v1"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/cniclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

const maxRetries = 2

type DynamicAttachmentRequestType string

type DynamicAttachmentRequest struct {
	PodName         string
	PodNamespace    string
	AttachmentNames []types.NetworkSelectionElement
	Type            DynamicAttachmentRequestType
}

// PodNetworksController handles the cncf networks annotations update, and
// triggers adding / removing networks from a running pod.
type PodNetworksController struct {
	k8sClientSet            kubernetes.Interface
	arePodsSynched          cache.InformerSynced
	areNetAttachDefsSynched cache.InformerSynced
	podsInformer            cache.SharedIndexInformer
	netAttachDefInformer    cache.SharedIndexInformer
	podsLister              v1corelisters.PodLister
	netAttachDefLister      nadlisterv1.NetworkAttachmentDefinitionLister
	broadcaster             record.EventBroadcaster
	recorder                record.EventRecorder
	workqueue               workqueue.RateLimitingInterface
	containerRuntime        containerruntimes.ContainerRuntime
	cniPlugin               cniclient.Client
	confDir                 string
	nadClientSet            nadclient.Interface
}

// NewPodNetworksController returns new PodNetworksController instance
func NewPodNetworksController(
	k8sCoreInformerFactory v1coreinformerfactory.SharedInformerFactory,
	nadInformers nadinformers.SharedInformerFactory,
	broadcaster record.EventBroadcaster,
	recorder record.EventRecorder,
	cniPlugin cniclient.Client,
	confDir string,
	k8sClientSet kubernetes.Interface,
	nadClientSet nadclient.Interface,
	containerRuntime containerruntimes.ContainerRuntime,
) (*PodNetworksController, error) {
	podInformer := k8sCoreInformerFactory.Core().V1().Pods().Informer()
	nadInformer := nadInformers.K8sCniCncfIo().V1().NetworkAttachmentDefinitions().Informer()

	podNetworksController := &PodNetworksController{
		arePodsSynched:          podInformer.HasSynced,
		areNetAttachDefsSynched: nadInformer.HasSynced,
		podsInformer:            podInformer,
		podsLister:              k8sCoreInformerFactory.Core().V1().Pods().Lister(),
		netAttachDefInformer:    nadInformer,
		netAttachDefLister:      nadInformers.K8sCniCncfIo().V1().NetworkAttachmentDefinitions().Lister(),
		recorder:                recorder,
		broadcaster:             broadcaster,
		containerRuntime:        containerRuntime,
		cniPlugin:               cniPlugin,
		confDir:                 confDir,
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"pod-networks-updates"),
		k8sClientSet: k8sClientSet,
		nadClientSet: nadClientSet,
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

	if ok := cache.WaitForCacheSync(stopChan, pnc.arePodsSynched, pnc.areNetAttachDefsSynched); !ok {
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
	queueItem, shouldQuit := pnc.workqueue.Get()
	if shouldQuit {
		return false
	}
	defer pnc.workqueue.Done(queueItem)

	dynAttachmentRequest := queueItem.(*DynamicAttachmentRequest)
	logging.Verbosef("extracted request [%v] from the queue", dynAttachmentRequest)
	err := pnc.handleDynamicInterfaceRequest(dynAttachmentRequest)
	pnc.handleResult(err, dynAttachmentRequest)

	return true
}

func (pnc *PodNetworksController) handleDynamicInterfaceRequest(dynamicAttachmentRequest *DynamicAttachmentRequest) error {
	logging.Verbosef("handleDynamicInterfaceRequest: read from queue: %v", dynamicAttachmentRequest)
	if dynamicAttachmentRequest.Type == "add" {
		pod, err := pnc.podsLister.Pods(dynamicAttachmentRequest.PodNamespace).Get(dynamicAttachmentRequest.PodName)
		if err != nil {
			return err
		}
		return pnc.addNetworks(dynamicAttachmentRequest.AttachmentNames, pod)
	} else if dynamicAttachmentRequest.Type == "remove" {
		pod, err := pnc.podsLister.Pods(dynamicAttachmentRequest.PodNamespace).Get(dynamicAttachmentRequest.PodName)
		if err != nil {
			return err
		}
		return pnc.removeNetworks(dynamicAttachmentRequest.AttachmentNames, pod)
	} else {
		logging.Verbosef("very weird attachment request: %+v", dynamicAttachmentRequest)
	}
	logging.Verbosef("handleDynamicInterfaceRequest: exited & successfully processed: %v", dynamicAttachmentRequest)
	return nil
}

func (pnc *PodNetworksController) handleResult(err error, dynamicAttachmentRequest *DynamicAttachmentRequest) {
	if err == nil {
		pnc.workqueue.Forget(dynamicAttachmentRequest)
		return
	}

	currentRetries := pnc.workqueue.NumRequeues(dynamicAttachmentRequest)
	if currentRetries <= maxRetries {
		_ = logging.Errorf("re-queued request for: %v", dynamicAttachmentRequest)
		pnc.workqueue.AddRateLimited(dynamicAttachmentRequest)
		return
	}

	pnc.workqueue.Forget(dynamicAttachmentRequest)
}

func (pnc *PodNetworksController) handlePodUpdate(oldObj interface{}, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	const (
		add    DynamicAttachmentRequestType = "add"
		remove DynamicAttachmentRequestType = "remove"
	)

	if reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
		return
	}
	podNamespace := oldPod.GetNamespace()
	podName := oldPod.GetName()
	logging.Debugf("pod [%s] updated", namespacedName(podNamespace, podName))

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
	logging.Verbosef("%d attachments to add to pod %s", len(toAdd), namespacedName(podNamespace, podName))
	if len(toAdd) > 0 {
		pnc.workqueue.Add(
			&DynamicAttachmentRequest{
				PodName:         podName,
				PodNamespace:    podNamespace,
				AttachmentNames: toAdd,
				Type:            add,
			})
	}

	toRemove := exclusiveNetworks(oldNetworkSelectionElements, newNetworkSelectionElements)
	logging.Verbosef("%d attachments to remove from pod %s", len(toRemove), namespacedName(podNamespace, podName))
	if len(toRemove) > 0 {
		pnc.workqueue.Add(
			&DynamicAttachmentRequest{
				PodName:         podName,
				PodNamespace:    podNamespace,
				AttachmentNames: toRemove,
				Type:            remove,
			})
	}
}

func namespacedName(podNamespace string, podName string) string {
	return fmt.Sprintf("%s/%s", podNamespace, podName)
}

func (pnc *PodNetworksController) addNetworks(netsToAdd []types.NetworkSelectionElement, pod *corev1.Pod) error {
	for _, netToAdd := range netsToAdd {
		cniParams, err := pnc.getCNIParams(pod, netToAdd)
		if err != nil {
			return logging.Errorf("failed to extract CNI params to hotplug new interface for pod %s: %v", pod.GetName(), err)
		}
		logging.Verbosef("CNI params for pod %s: %+v", pod.GetName(), cniParams)

		delegateConf, _, err := pnc.GetKubernetesDelegate(&netToAdd, pnc.confDir, pod, nil)
		if err != nil {
			return fmt.Errorf("error retrieving the delegate info: %w", err)
		}

		k8sArgs, _ := k8sclient.GetK8sArgs(&cniParams.CniCmdArgs)
		multusNetconf, err := pnc.multusConf()
		if err != nil {
			return logging.Errorf("failed to retrieve the multus network: %v", err)
		}

		result, err := pnc.cniPlugin.AddNetworks(pnc.k8sClient(), pod, &cniParams.CniCmdArgs, k8sArgs, delegateConf, cniParams.BuildRuntimeConf(), multusNetconf)
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
	podNetStatus = make([]nadv1.NetworkStatus, 0) // SetPodNetworkStatusAnnotation won't set the status if this is nil
	for _, netToRemove := range netsToRemove {
		cniParams, err := pnc.getCNIParams(pod, netToRemove)
		if err != nil {
			return logging.Errorf("failed to extract CNI params to remove existing interface from pod %s: %v", pod.GetName(), err)
		}
		logging.Verbosef("CNI params for pod %s: %+v", pod.GetName(), cniParams)

		delegateConf, _, err := pnc.GetKubernetesDelegate(&netToRemove, pnc.confDir, pod, nil)
		if err != nil {
			return fmt.Errorf("error retrieving the delegate info: %w", err)
		}
		k8sArgs, _ := k8sclient.GetK8sArgs(&cniParams.CniCmdArgs)

		multusNetconf, err = pnc.multusConf()
		if err != nil {
			return logging.Errorf("failed to retrieve the multus network: %v", err)
		}

		rtConf := types.DelegateRuntimeConfig(cniParams.CniCmdArgs.ContainerID, delegateConf, multusNetconf.RuntimeConfig, cniParams.CniCmdArgs.IfName)

		if err := pnc.cniPlugin.RemoveNetworks(pod, &cniParams.CniCmdArgs, k8sArgs, delegateConf, rtConf, multusNetconf); err != nil {
			return logging.Errorf("failed to remove network. error: %v", err)
		}
		logging.Verbosef("removed network %s from pod %s with interface name: %s", netToRemove.Name, pod.GetName(), cniParams.CniCmdArgs.IfName)

		oldNetStatus, err := networkStatus(pod.Annotations)
		if err != nil {
			return fmt.Errorf("failed to extract the pod's network status: %+v", err)
		}

		for _, netStatus := range oldNetStatus {
			if namespacedName(netToRemove.Namespace, netToRemove.Name) != netStatus.Name {
				podNetStatus = append(podNetStatus, netStatus)
			}
		}
	}

	if multusNetconf == nil {
		var err error
		multusNetconf, err = pnc.multusConf()
		if err != nil {
			logging.Verbosef("error accessing the multus configuration: %+v", err)
		}
	}

	if err := k8sclient.SetPodNetworkStatusAnnotation(pnc.k8sClient(), pod.GetName(), pod.GetNamespace(), string(pod.GetUID()), podNetStatus, multusNetconf); err != nil {
		// error happen but continue to delete
		logging.Errorf("Multus: error unsetting the networks status: %v", err)
	}

	return nil
}

func (pnc *PodNetworksController) k8sClient() *k8sclient.ClientInfo {
	return &k8sclient.ClientInfo{
		Client:           pnc.k8sClientSet,
		NetClient:        pnc.nadClientSet.K8sCniCncfIoV1(),
		EventBroadcaster: nil,
		EventRecorder:    pnc.recorder,
	}
}

func networkSelectionElements(podAnnotations map[string]string, podNamespace string) ([]*types.NetworkSelectionElement, error) {
	podNetworks, ok := podAnnotations[nadv1.NetworkAttachmentAnnot]
	if !ok {
		return nil, fmt.Errorf("the pod is missing the \"%s\" annotation on its annotations: %+v", nadv1.NetworkAttachmentAnnot, podAnnotations)
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
		return nil, fmt.Errorf("the pod is missing the \"%s\" annotation on its annotations: %+v", nadv1.NetworkStatusAnnot, podAnnotations)
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
			Path:        strings.Join(pnc.cniPlugin.PluginPath(), ";"), // TODO: what to put here ?
			StdinData:   nil,                                           // TODO: what to put here ?
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

// GetKubernetesDelegate uses the pod controller net-attach-def lister to retrieve the attachment info and parse it into
// a kubernetes delegate.
func (pnc *PodNetworksController) GetKubernetesDelegate(net *types.NetworkSelectionElement, confdir string, pod *corev1.Pod, resourceMap map[string]*types.ResourceInfo) (*types.DelegateNetConf, map[string]*types.ResourceInfo, error) {
	logging.Debugf("GetKubernetesDelegate: %v, %s, %v, %v", *net, confdir, *pod, resourceMap)
	customResource, err := pnc.netAttachDefLister.NetworkAttachmentDefinitions(net.Namespace).Get(net.Name)
	if err != nil {
		errMsg := fmt.Sprintf("cannot find a network-attachment-definition (%s) in namespace (%s): %v", net.Name, net.Namespace, err)
		pnc.Eventf(pod, corev1.EventTypeWarning, "NoNetworkFound", errMsg)
		return nil, resourceMap, logging.Errorf("GetKubernetesDelegate: " + errMsg)
	}

	return k8sclient.K8sDelegate(customResource, pod, resourceMap, confdir, net)
}

// Eventf puts event into kubernetes events
func (pnc *PodNetworksController) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if pnc != nil && pnc.recorder != nil {
		pnc.recorder.Eventf(object, eventtype, reason, messageFmt, args...)
	}
}
