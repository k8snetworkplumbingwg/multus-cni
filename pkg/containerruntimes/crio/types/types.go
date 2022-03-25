package types

// PodStatusResponseInfo represents the container status reply - crictl ps <containerID>
type PodStatusResponseInfo struct {
	SandboxID   string
	RunTimeSpec RunTimeSpecInfo
}

// RunTimeSpecInfo represents the relevant part of the container status spec
type RunTimeSpecInfo struct {
	Linux NamespacesInfo
}

// NamespacesInfo represents the container status namespaces
type NamespacesInfo struct {
	Namespaces []NameSpaceInfo
}

// NameSpaceInfo represents the ns info
type NameSpaceInfo struct {
	Type string
	Path string
}
