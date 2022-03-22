package cniclient

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCniClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CNI client suite")
}

var _ = Describe("CNI client", func() {
	Context("", func() {
		BeforeEach(func() {

		})

		AfterEach(func() {

		})

		It("", func() {

		})
	})
})
