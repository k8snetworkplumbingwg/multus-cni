package fake

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/gogo/protobuf/types"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type Client struct {
	cache map[string]containerd.Container
}

type ClientOpt func(client *Client)

func NewFakeClient(opts ...ClientOpt) *Client {
	client := &Client{cache: map[string]containerd.Container{}}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func WithCachedContainer(containerID string, container containerd.Container) ClientOpt {
	return func(client *Client) {
		client.cache[containerID] = container
	}
}

func (c Client) LoadContainer(_ context.Context, id string) (containerd.Container, error) {
	container, wasFound := c.cache[id]
	if wasFound {
		return container, nil
	}
	return nil, fmt.Errorf("container not found: %s", id)
}

type Container struct {
	id               string
	missingLinuxInfo bool
	netnsPath        string
}

func NewFakeContainer(id string, netnsPath string) *Container {
	return &Container{
		id:        id,
		netnsPath: netnsPath,
	}
}

func NewFakeContainerWithoutNetworkNamespace(id string) *Container {
	return &Container{
		id: id,
	}
}

func NewFakeNonLinuxContainer(id string) *Container {
	return &Container{
		id:               id,
		missingLinuxInfo: true,
	}
}

func (c Container) ID() string {
	return c.id
}

func (Container) Info(ctx context.Context, opts ...containerd.InfoOpts) (containers.Container, error) {
	return containers.Container{}, nil
}

func (Container) Delete(ctx context.Context, opts ...containerd.DeleteOpts) error {
	return nil
}

func (Container) NewTask(ctx context.Context, creator cio.Creator, opts ...containerd.NewTaskOpts) (containerd.Task, error) {
	return nil, nil
}

func (c Container) Spec(ctx context.Context) (*oci.Spec, error) {
	if c.missingLinuxInfo {
		return &oci.Spec{}, nil
	}
	if c.netnsPath == "" {
		return &oci.Spec{Linux: &specs.Linux{}}, nil
	}
	return &oci.Spec{
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.NetworkNamespace, Path: c.netnsPath},
			},
		},
	}, nil
}

func (Container) Task(ctx context.Context, attach cio.Attach) (containerd.Task, error) {
	return nil, nil
}

func (Container) Image(ctx context.Context) (containerd.Image, error) {
	return nil, nil
}

func (Container) Labels(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (Container) SetLabels(ctx context.Context, m map[string]string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (Container) Extensions(ctx context.Context) (map[string]types.Any, error) {
	return map[string]types.Any{}, nil
}

func (Container) Update(ctx context.Context, opts ...containerd.UpdateContainerOpts) error {
	return nil
}

func (Container) Checkpoint(ctx context.Context, s string, opts ...containerd.CheckpointOpts) (containerd.Image, error) {
	return nil, nil
}
