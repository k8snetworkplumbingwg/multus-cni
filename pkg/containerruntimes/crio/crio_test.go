package crio

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes/crio/fake"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic network attachment controller suite")
}

var _ = Describe("CRI-O runtime", func() {
	var runtime *CrioRuntime

	When("the runtime *does not* feature any containers", func() {
		BeforeEach(func() {
			runtime = newDummyCrioRuntime()
		})

		It("cannot extract the network namespace of a container", func() {
			_, err := runtime.NetNS("1234")
			Expect(err).To(MatchError("failed to get pod sandbox info: container 1234 not found"))
		})
	})

	When("a live container is provisioned in the runtime", func() {
		const (
			containerID = "1234"
			netnsPath   = "bottom-drawer"
		)
		BeforeEach(func() {
			runtime = newDummyCrioRuntime(fake.WithCachedContainer(containerID, netnsPath))
		})

		It("cannot extract the network namespace of a container", func() {
			Expect(runtime.NetNS(containerID)).To(Equal(netnsPath))
		})
	})
})

func newDummyCrioRuntime(opts ...fake.ClientOpt) *CrioRuntime {
	runtimeClient := fake.NewFakeClient()

	for _, opt := range opts {
		opt(runtimeClient)
	}

	ctx := context.TODO()
	ctxWithCancel, cancelFunc := context.WithCancel(ctx)
	return &CrioRuntime{
		client:     runtimeClient,
		context:    ctxWithCancel,
		cancelFunc: cancelFunc,
	}
}
