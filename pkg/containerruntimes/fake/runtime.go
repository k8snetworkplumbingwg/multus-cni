package fake

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	v1 "k8s.io/api/core/v1"
)

type Runtime struct {
	cache map[string]string
}

func NewFakeRuntime(pods ...v1.Pod) *Runtime {
	runtimeCache := map[string]string{}

	for _, pod := range pods {
		hash := md5.Sum([]byte(pod.GetName()))
		runtimeCache[pod.GetName()] = hex.EncodeToString(hash[:])
	}
	return &Runtime{cache: runtimeCache}
}

func (r *Runtime) NetNS(containerID string) (string, error) {
	if netnsName, wasFound := r.cache[containerID]; wasFound {
		return netnsName, nil
	}
	return "", fmt.Errorf("could not find a network namespace for container: %s", containerID)
}
