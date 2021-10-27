package controller

import (
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

const allNamespaces = ""
const controllerName = "multus-cni-pod-networks-controller"
const podNetworksAnnot = "k8s.v1.cni.cncf.io/networks"

type CNIParams struct {
	Namespace   string
	PodName     string
	SandboxID   string
	NetnsPath   string
	NetworkName string
	IfMAC       string
}

// PodNetworksController handles the cncf networks annotations update, and
// triggers adding / removing networks from a running pod.
type PodNetworksController struct {
	k8sClientSet kubernetes.Interface
	podsSynced   cache.InformerSynced
	workqueue    workqueue.RateLimitingInterface
	recorder     record.EventRecorder
}

// NewPodNetworksController returns new PodNetworksController instance
func NewPodNetworksController(
	k8sClientSet kubernetes.Interface,
	podInformer cache.SharedIndexInformer) (*PodNetworksController, error) {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logging.Verbosef)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: k8sClientSet.CoreV1().Events(allNamespaces)})

	podNetworksController := &PodNetworksController{
		k8sClientSet: k8sClientSet,
		podsSynced:   podInformer.HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"pod-networks-updates"),
		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName}),
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

	podNamespace := oldPod.GetNamespace()
	podName := oldPod.GetName()
	logging.Debugf("pod [%s] update", fmt.Sprintf("%s/%s", podNamespace, podName))

	if reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
		return
	}

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
	// to remove
	toRemove := exclusiveNetworks(oldNetworkSelectionElements, newNetworkSelectionElements)
	logging.Verbosef("pods to remove: %+v", toRemove)
	// to add
	toAdd := exclusiveNetworks(newNetworkSelectionElements, oldNetworkSelectionElements)
	logging.Verbosef("pods to add: %+v", toAdd)
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

func exclusiveNetworks(needles []*types.NetworkSelectionElement, haystack []*types.NetworkSelectionElement) []types.NetworkSelectionElement {
	setOfNeedles := listToSet(needles)
	haystackSet := listToSet(haystack)

	var unmatchedNetworks []types.NetworkSelectionElement
	for needleNetName, needle := range setOfNeedles {
		if _, ok := haystackSet[needleNetName]; !ok {
			unmatchedNetworks = append(unmatchedNetworks, needle)
		}
	}
	return unmatchedNetworks
}

func listToSet(list []*types.NetworkSelectionElement) map[string]types.NetworkSelectionElement {
	set := make(map[string]types.NetworkSelectionElement) // New empty set
	for k := range list {
		set[list[k].Name] = *list[k]
	}
	return set
}
