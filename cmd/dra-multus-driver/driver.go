package main

import (
	"context"
	"fmt"

	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/utils/ptr"
)

var _ drapbv1.DRAPluginServer = &driver{}

type driver struct {
	client coreclientset.Interface
	plugin kubeletplugin.DRAPlugin
	state  *DeviceState
}

func NewDriver(ctx context.Context, config *Config) (*driver, error) {
	driver := &driver{
		client: config.coreclient,
	}

	// Initialize device state
	state, err := NewDeviceState(config)
	if err != nil {
		return nil, err
	}
	driver.state = state

	// Start the DRA plugin
	plugin, err := kubeletplugin.Start(
		ctx,
		[]any{driver},
		kubeletplugin.KubeClient(config.coreclient),
		kubeletplugin.NodeName(config.flags.nodeName),
		kubeletplugin.DriverName(DriverName),
		kubeletplugin.RegistrarSocketPath(PluginRegistrationPath),
		kubeletplugin.PluginSocketPath(DriverPluginSocketPath),
		kubeletplugin.KubeletPluginSocketPath(DriverPluginSocketPath),
	)
	if err != nil {
		return nil, err
	}
	driver.plugin = plugin

	// ✅ Publish one dummy allocatable device to advertise this driver
	resources := kubeletplugin.Resources{
		Devices: []resourceapi.Device{
			{
				Name: "net0",
				Basic: &resourceapi.BasicDevice{
					Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
						"driver": {
							StringValue: ptr.To(DriverName),
						},
					},
				},
			},
		},
	}

	if err := plugin.PublishResources(ctx, resources); err != nil {
		return nil, fmt.Errorf("failed to publish resources: %w", err)
	}

	klog.Infof("Successfully registered driver and published resources")
	return driver, nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	d.plugin.Stop()
	return nil
}

func (d *driver) NodePrepareResources(ctx context.Context, req *drapbv1.NodePrepareResourcesRequest) (*drapbv1.NodePrepareResourcesResponse, error) {
	klog.Infof("NodePrepareResources called: number of claims: %d", len(req.Claims))
	prepared := &drapbv1.NodePrepareResourcesResponse{
		Claims: map[string]*drapbv1.NodePrepareResourceResponse{},
	}

	for _, claim := range req.Claims {
		prepared.Claims[claim.UID] = d.nodePrepareResource(ctx, claim)
	}

	return prepared, nil
}

func (d *driver) nodePrepareResource(ctx context.Context, claim *drapbv1.Claim) *drapbv1.NodePrepareResourceResponse {
	resourceClaim, err := d.client.ResourceV1beta1().ResourceClaims(claim.Namespace).Get(
		ctx,
		claim.Name,
		metav1.GetOptions{})
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("failed to fetch ResourceClaim %q in namespace %q: %v", claim.Name, claim.Namespace, err),
		}
	}

	devices, err := d.state.Prepare(resourceClaim)
	if err != nil {
		return &drapbv1.NodePrepareResourceResponse{
			Error: fmt.Sprintf("error preparing devices for claim %q: %v", claim.UID, err),
		}
	}

	klog.Infof("Prepared NAD-based device for claim %q: %+v", claim.UID, devices)
	return &drapbv1.NodePrepareResourceResponse{
		Devices: devices,
	}
}

func (d *driver) NodeUnprepareResources(ctx context.Context, req *drapbv1.NodeUnprepareResourcesRequest) (*drapbv1.NodeUnprepareResourcesResponse, error) {
	klog.Infof("NodeUnprepareResources called: number of claims: %d", len(req.Claims))
	unprepared := &drapbv1.NodeUnprepareResourcesResponse{
		Claims: map[string]*drapbv1.NodeUnprepareResourceResponse{},
	}

	for _, claim := range req.Claims {
		unprepared.Claims[claim.UID] = d.nodeUnprepareResource(ctx, claim)
	}

	return unprepared, nil
}

func (d *driver) nodeUnprepareResource(ctx context.Context, claim *drapbv1.Claim) *drapbv1.NodeUnprepareResourceResponse {
	if err := d.state.Unprepare(claim.UID); err != nil {
		return &drapbv1.NodeUnprepareResourceResponse{
			Error: fmt.Sprintf("error unpreparing devices for claim %q: %v", claim.UID, err),
		}
	}

	return &drapbv1.NodeUnprepareResourceResponse{}
}
