package containerd

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const k8sNamespace = "k8s.io"

// ContainerdRuntime represents a connection to the containerd runtime
type ContainerdRuntime struct {
	client            Client
	namespacedContext context.Context
}

// NewContainerdRuntime connects to the containerd runtime over the specified `socketPath`
func NewContainerdRuntime(socketPath string, timeout time.Duration) (*ContainerdRuntime, error) {
	containerdRuntime, err := containerd.New(
		socketPath,
		containerd.WithTimeout(timeout),
		containerd.WithDefaultNamespace(k8sNamespace))
	if err != nil {
		return nil, fmt.Errorf("failed to create containerd client: %w", err)
	}

	return newContainerdRuntime(containerdRuntime), nil
}

func newContainerdRuntime(client Client) *ContainerdRuntime {
	return &ContainerdRuntime{
		client:            client,
		namespacedContext: namespaces.WithNamespace(context.Background(), k8sNamespace),
	}
}

// NetNS returns the netns path of a given container
func (cd *ContainerdRuntime) NetNS(containerID string) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("ID cannot be empty")
	}

	containerSpec, err := cd.containerSpec(containerID)
	if err != nil {
		return "", err
	}

	if containerSpec.Linux == nil {
		return "", fmt.Errorf("container does not feature platform-specific configuration for Linux based containers")
	}

	for _, ns := range containerSpec.Linux.Namespaces {
		if ns.Type == specs.NetworkNamespace {
			return ns.Path, nil
		}
	}
	return "", fmt.Errorf("could not find netns for container ID: %s", containerID)
}

func (cd *ContainerdRuntime) containerSpec(containerID string) (*oci.Spec, error) {
	container, err := cd.client.LoadContainer(cd.namespacedContext, containerID)
	if err != nil {
		return nil, err
	}
	return container.Spec(cd.namespacedContext)
}
