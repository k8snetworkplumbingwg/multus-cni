package containerruntimes

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
