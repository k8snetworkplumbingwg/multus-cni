package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containernetworking/cni/libcni"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"

	netclientset "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	configapi "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/dra/api/multus-cni.io/resource/net/v1alpha1"
	multusk8sutils "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"k8s.io/klog/v2"
)

type AllocatableDevices map[string]resourceapi.Device
type PreparedDevices []*PreparedDevice
type PreparedClaims map[string]PreparedDevices
type PerDeviceCDIContainerEdits map[string]*cdiapi.ContainerEdits

type OpaqueDeviceConfig struct {
	Requests []string
	Config   runtime.Object
}

type PreparedDevice struct {
	drapbv1.Device
	ContainerEdits *cdiapi.ContainerEdits
}

func (pds PreparedDevices) GetDevices() []*drapbv1.Device {
	var devices []*drapbv1.Device
	for _, pd := range pds {
		devices = append(devices, &pd.Device)
	}
	return devices
}

type DeviceState struct {
	sync.Mutex
	cdi               *CDIHandler
	checkpointManager checkpointmanager.CheckpointManager
	nadClient         netclientset.Interface
}

func NewDeviceState(config *Config) (*DeviceState, error) {

	cdi, err := NewCDIHandler(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI handler: %v", err)
	}
	if err := cdi.CreateCommonSpecFile(); err != nil {
		return nil, fmt.Errorf("unable to create CDI common spec file: %v", err)
	}

	checkpointManager, err := checkpointmanager.NewCheckpointManager(DriverPluginPath)
	if err != nil {
		return nil, fmt.Errorf("unable to create checkpoint manager: %v", err)
	}

	clientconfig, err := config.flags.kubeClientConfig.NewClientSetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to create clientset config: %v", err)
	}
	nadClient, err := netclientset.NewForConfig(clientconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create NAD client: %v", err)
	}

	checkpoints, err := checkpointManager.ListCheckpoints()
	if err != nil {
		return nil, fmt.Errorf("unable to list checkpoints: %v", err)
	}

	for _, c := range checkpoints {
		if c == DriverPluginCheckpointFile {
			return &DeviceState{
				cdi:               cdi,
				checkpointManager: checkpointManager,
				nadClient:         nadClient,
			}, nil
		}
	}

	// If checkpoint wasn't found, create a new one
	checkpoint := newCheckpoint()
	if err := checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to create initial checkpoint: %v", err)
	}

	return &DeviceState{
		cdi:               cdi,
		checkpointManager: checkpointManager,
		nadClient:         nadClient,
	}, nil
}

func (s *DeviceState) Prepare(claim *resourceapi.ResourceClaim) ([]*drapbv1.Device, error) {
	s.Lock()
	defer s.Unlock()

	claimUID := string(claim.UID)

	checkpoint := newCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] != nil {
		return preparedClaims[claimUID].GetDevices(), nil
	}

	preparedDevices, err := s.prepareDevices(claim)
	if err != nil {
		return nil, fmt.Errorf("prepare failed: %v", err)
	}

	if err = s.cdi.CreateClaimSpecFile(claimUID, preparedDevices); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	preparedClaims[claimUID] = preparedDevices
	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return preparedClaims[claimUID].GetDevices(), nil
}

func (s *DeviceState) Unprepare(claimUID string) error {
	s.Lock()
	defer s.Unlock()

	checkpoint := newCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] == nil {
		return nil
	}

	if err := s.unprepareDevices(claimUID, preparedClaims[claimUID]); err != nil {
		return fmt.Errorf("unprepare failed: %v", err)
	}

	err := s.cdi.DeleteClaimSpecFile(claimUID)
	if err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %v", err)
	}

	delete(preparedClaims, claimUID)
	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return nil
}

func (s *DeviceState) prepareDevices(claim *resourceapi.ResourceClaim) (PreparedDevices, error) {
	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim not yet allocated")
	}

	// Sanity: should only have one opaque config per claim
	if len(claim.Status.Allocation.Devices.Config) == 0 {
		return nil, fmt.Errorf("no config provided in claim allocation")
	}

	opaque := claim.Status.Allocation.Devices.Config[0].DeviceConfiguration.Opaque
	if opaque == nil || opaque.Driver != DriverName {
		return nil, fmt.Errorf("claim does not contain expected opaque config for driver %q", DriverName)
	}

	obj, err := runtime.Decode(configapi.Decoder, opaque.Parameters.Raw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode opaque config: %w", err)
	}

	netConfig, ok := obj.(*configapi.NetConfig)
	if !ok {
		return nil, fmt.Errorf("decoded opaque config is not a *NetConfig")
	}

	// Apply CDI edits to all results using this config
	var results []*resourceapi.DeviceRequestAllocationResult
	for i := range claim.Status.Allocation.Devices.Results {
		results = append(results, &claim.Status.Allocation.Devices.Results[i])
	}

	var podName, podNamespace string

	for _, owner := range claim.OwnerReferences {
		if owner.Kind == "Pod" && owner.Name != "" {
			podName = owner.Name
			podNamespace = claim.Namespace
			break
		}
	}

	if podName == "" {
		return nil, fmt.Errorf("could not determine owning pod from claim metadata")
	}

	perDeviceCDIContainerEdits, err := s.applyConfig(netConfig, results, podName, podNamespace, string(claim.UID))

	if err != nil {
		return nil, fmt.Errorf("failed to apply CDI container edits: %w", err)
	}

	var preparedDevices PreparedDevices
	for _, result := range claim.Status.Allocation.Devices.Results {
		device := &PreparedDevice{
			Device: drapbv1.Device{
				RequestNames: []string{result.Request},
				PoolName:     result.Pool,
				DeviceName:   result.Device,
				CDIDeviceIDs: s.cdi.GetClaimDevices(string(claim.UID), []string{result.Device}),
			},
			ContainerEdits: perDeviceCDIContainerEdits[result.Device],
		}
		preparedDevices = append(preparedDevices, device)
	}

	return preparedDevices, nil
}

func (s *DeviceState) applyConfig(
	config *configapi.NetConfig,
	results []*resourceapi.DeviceRequestAllocationResult,
	podName string,
	podNamespace string,
	claimUID string,
) (PerDeviceCDIContainerEdits, error) {
	perDeviceEdits := make(PerDeviceCDIContainerEdits)
	var delegates []*types.DelegateNetConf

	// --- Add the cluster default network as the first delegate ---
	multusConfPath := "/host/etc/cni/net.d/00-multus.conf"
	multusConfBytes, err := os.ReadFile(multusConfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read multus config from %s: %w", multusConfPath, err)
	}

	var multusConf struct {
		ClusterNetwork string `json:"clusterNetwork"`
	}
	if err := json.Unmarshal(multusConfBytes, &multusConf); err != nil {
		return nil, fmt.Errorf("failed to parse multus config json: %w", err)
	}
	if multusConf.ClusterNetwork == "" {
		return nil, fmt.Errorf("no clusterNetwork field found in multus config")
	}

	// Load the default network CNI config
	var defaultConfBytes []byte
	isconflist := false
	if strings.HasSuffix(multusConf.ClusterNetwork, ".conflist") {
		confList, err := libcni.ConfListFromFile(multusConf.ClusterNetwork)
		if err != nil {
			return nil, fmt.Errorf("failed to load CNI conflist from %s: %w", multusConf.ClusterNetwork, err)
		}
		isconflist = true
		defaultConfBytes = confList.Bytes
	} else {
		conf, err := libcni.ConfFromFile(multusConf.ClusterNetwork)
		if err != nil {
			return nil, fmt.Errorf("failed to load CNI config from %s: %w", multusConf.ClusterNetwork, err)
		}
		if conf.Network.Type == "" {
			return nil, fmt.Errorf("CNI config in %s missing type field", multusConf.ClusterNetwork)
		}
		defaultConfBytes = conf.Bytes
	}

	// Create and append the default delegate
	defaultDelegate, err := types.LoadDelegateNetConf(defaultConfBytes, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to load delegate from default CNI config: %w", err)
	}
	defaultDelegate.MasterPlugin = true
	defaultDelegate.ConfListPlugin = isconflist
	delegates = append(delegates, defaultDelegate)

	// --- Add the user-defined network attachments ---
	parsedNets, err := multusk8sutils.ParsePodNetworkAnnotation(config.Networks, podNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to parse networks string: %w", err)
	}

	for _, net := range parsedNets {
		nad, err := s.nadClient.K8sCniCncfIoV1().NetworkAttachmentDefinitions(net.Namespace).Get(context.TODO(), net.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch NAD %s/%s: %w", net.Namespace, net.Name, err)
		}

		delegate, err := types.LoadDelegateNetConf([]byte(nad.Spec.Config), net, "", "")
		if err != nil {
			return nil, fmt.Errorf("failed to load delegate netconf from NAD %s/%s: %w", net.Namespace, net.Name, err)
		}

		if net.InterfaceRequest != "" {
			delegate.IfnameRequest = net.InterfaceRequest
		}
		delegates = append(delegates, delegate)
	}

	klog.Infof("Delegate information for pod %s/%s: %+v", podNamespace, podName, delegates)

	// Save delegates to a file
	delegatesPath := filepath.Join("/run/k8s.cni.cncf.io/dra", fmt.Sprintf("%s_%s.json", podNamespace, podName))
	if err := os.MkdirAll(filepath.Dir(delegatesPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to ensure delegate output dir: %w", err)
	}
	data, err := json.MarshalIndent(delegates, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal delegates: %w", err)
	}
	if err := os.WriteFile(delegatesPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write delegates file: %w", err)
	}

	// Add CDI env vars
	for _, result := range results {
		envs := []string{
			fmt.Sprintf("MULTUS_DRA_DEVICE_NAME=%s", result.Device),
			fmt.Sprintf("MULTUS_DRA_NETWORKS=%s", config.Networks),
			fmt.Sprintf("MULTUS_DRA_POD_NAMESPACE=%s", podNamespace),
			fmt.Sprintf("MULTUS_DRA_POD_NAME=%s", podName),
			fmt.Sprintf("MULTUS_DRA_CLAIM_UID=%s", claimUID),
		}

		for i, net := range parsedNets {
			envs = append(envs, fmt.Sprintf("MULTUS_DRA_NET_%d_NAME=%s", i, net.Name))
			envs = append(envs, fmt.Sprintf("MULTUS_DRA_NET_%d_NAMESPACE=%s", i, net.Namespace))
			envs = append(envs, fmt.Sprintf("MULTUS_DRA_NET_%d_IFNAME=%s", i, net.InterfaceRequest))
		}

		perDeviceEdits[result.Device] = &cdiapi.ContainerEdits{
			ContainerEdits: &cdispec.ContainerEdits{Env: envs},
		}
	}

	return perDeviceEdits, nil
}

func GetOpaqueDeviceConfig(
	decoder runtime.Decoder,
	driverName string,
	possibleConfigs []resourceapi.DeviceAllocationConfiguration,
) (*configapi.NetConfig, error) {
	for _, config := range possibleConfigs {
		if config.DeviceConfiguration.Opaque == nil {
			continue
		}
		if config.DeviceConfiguration.Opaque.Driver != driverName {
			continue
		}
		decoded, err := runtime.Decode(decoder, config.DeviceConfiguration.Opaque.Parameters.Raw)
		if err != nil {
			return nil, fmt.Errorf("error decoding opaque config: %w", err)
		}
		netConfig, ok := decoded.(*configapi.NetConfig)
		if !ok {
			return nil, fmt.Errorf("decoded config is not of type *NetConfig")
		}
		return netConfig, nil
	}
	return nil, fmt.Errorf("no matching opaque config found for driver %q", driverName)
}

func (s *DeviceState) unprepareDevices(claimUID string, devices PreparedDevices) error {
	return nil
}
