package containerd

import (
	"context"

	"github.com/containerd/containerd"
)

type Client interface {
	LoadContainer(ctx context.Context, id string) (containerd.Container, error)
}
