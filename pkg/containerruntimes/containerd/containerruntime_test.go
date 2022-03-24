package containerd

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes/containerd/fake"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic network attachment controller suite")
}

var _ = Describe("Container runtimes package", func() {
	var runtime *ContainerdRuntime

	When("a live container is provisioned in the runtime", func() {
		const (
			containerID = "1234"
			netnsPath   = "/tmp/below-the-desk"
		)

		BeforeEach(func() {
			runtime = newContainerdRuntime(newDummyContainerdRuntime(fake.WithCachedContainer(
				containerID,
				fake.NewFakeContainer(containerID, netnsPath))))
		})

		It("its network namespace is read when queried", func() {
			Expect(runtime.NetNS(containerID)).To(Equal(netnsPath))
		})

		It("cannot query an empty container ID", func() {
			const noID = ""
			_, err := runtime.NetNS(noID)
			Expect(err).To(MatchError("ID cannot be empty"))
		})
	})

	When("the runtime *does not* feature any containers", func() {
		BeforeEach(func() {
			runtime = newContainerdRuntime(newDummyContainerdRuntime())
		})

		It("cannot extract the network namespace of a container", func() {
			const wrongContainerID = "no-go"

			_, err := runtime.NetNS(wrongContainerID)
			expectedErrorString := fmt.Sprintf("container not found: %s", wrongContainerID)
			Expect(err).To(MatchError(expectedErrorString))
		})
	})

	When("the runtime features a non-linux container", func() {
		const containerID = "1234"

		BeforeEach(func() {
			runtime = newContainerdRuntime(newDummyContainerdRuntime(fake.WithCachedContainer(
				containerID,
				fake.NewFakeNonLinuxContainer(containerID))))
		})

		It("its network namespace is read when queried", func() {
			_, err := runtime.NetNS(containerID)
			Expect(err).To(
				MatchError(
					"container does not feature platform-specific configuration for Linux based containers"))
		})
	})

	When("the runtime features a container without network namespace", func() {
		const containerID = "1234"

		BeforeEach(func() {
			runtime = newContainerdRuntime(newDummyContainerdRuntime(fake.WithCachedContainer(
				containerID,
				fake.NewFakeContainerWithoutNetworkNamespace(containerID))))
		})

		It("its network namespace is read when queried", func() {
			_, err := runtime.NetNS(containerID)
			expectedErrorString := fmt.Sprintf("could not find netns for container ID: %s", containerID)
			Expect(err).To(MatchError(expectedErrorString))
		})
	})
})

func newDummyContainerdRuntime(opts ...fake.ClientOpt) Client {
	return fake.NewFakeClient(opts...)
}
