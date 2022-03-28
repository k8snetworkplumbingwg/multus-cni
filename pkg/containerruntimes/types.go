package containerruntimes

import (
	"fmt"
	"strings"
	"time"
)

// RuntimeType indicates the type of runtime
type RuntimeType int8

const (
	// Crio represents the CRI-O container runtime
	Crio RuntimeType = iota
	// Containerd represents the containerd container runtime
	Containerd
)

// ContainerRuntime interface
type ContainerRuntime interface {
	// NetNS returns the network namespace of the given containerID.
	NetNS(containerID string) (string, error)
}

// NewRuntime returns the correct runtime connection according to `RuntimeType`
func NewRuntime(socketPath string, runtimeType RuntimeType) (ContainerRuntime, error) {
	var runtime ContainerRuntime
	var err error

	switch runtimeType {
	case Crio:
		runtime, err = NewCrioRuntime(socketPath, 5*time.Second)
	case Containerd:
		runtime, err = NewContainerdRuntime(socketPath, time.Second)
	}

	if err != nil {
		return nil, err
	}
	return runtime, nil
}

func ParseRuntimeType(rt string) (RuntimeType, error) {
	const (
		crio       = "crio"
		containerd = "containerd"
	)
	if strings.ToLower(rt) == crio {
		return Crio, nil
	} else if strings.ToLower(rt) == containerd {
		return Containerd, nil
	}
	return Crio, fmt.Errorf("invalid runtime type: %s. Allowed values are: %s, %s", rt, crio, containerd)
}
