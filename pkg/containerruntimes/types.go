package containerruntimes

import "time"

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
func NewRuntime(socketPath string, runtimeType RuntimeType) (*ContainerRuntime, error) {
	var runtime ContainerRuntime
	var err error

	switch runtimeType {
	case Crio:
		// TODO
	case Containerd:
		runtime, err = NewContainerdRuntime(socketPath, time.Second)
	}

	if err != nil {
		return nil, err
	}
	return &runtime, nil
}
