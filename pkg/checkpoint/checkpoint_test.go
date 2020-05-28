package checkpoint

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"io/ioutil"
	"testing"

	"gopkg.in/intel/multus-cni.v3/pkg/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
)

const (
	fakeTempFile = "/tmp/kubelet_internal_checkpoint"
)

type fakeCheckpoint struct {
	fileName string
}

func (fc *fakeCheckpoint) WriteToFile(inBytes []byte) error {
	return ioutil.WriteFile(fc.fileName, inBytes, 0600)
}

func (fc *fakeCheckpoint) DeleteFile() error {
	return os.Remove(fc.fileName)
}

func TestCheckpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Checkpoint")
}

var _ = BeforeSuite(func() {
	sampleData := `{
		"Data": {
			"PodDeviceEntries": [
			{
				"PodUID": "970a395d-bb3b-11e8-89df-408d5c537d23",
				"ContainerName": "appcntr1",
				"ResourceName": "intel.com/sriov_net_A",
				"DeviceIDs": [
				"0000:03:02.3",
				"0000:03:02.0"
				],
				"AllocResp": "CikKC3NyaW92X25ldF9BEhogMDAwMDowMzowMi4zIDAwMDA6MDM6MDIuMA=="
			}
			],
			"RegisteredDevices": {
			"intel.com/sriov_net_A": [
				"0000:03:02.1",
				"0000:03:02.2",
				"0000:03:02.3",
				"0000:03:02.0"
			],
			"intel.com/sriov_net_B": [
				"0000:03:06.3",
				"0000:03:06.0",
				"0000:03:06.1",
				"0000:03:06.2"
			]
			}
		},
		"Checksum": 229855270
		}`

	fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
	err := fakeCheckpoint.WriteToFile([]byte(sampleData))
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Kubelet checkpoint data read operations", func() {
	Context("Using /tmp/kubelet_internal_checkpoint file", func() {
		var (
			cp            types.ResourceClient
			err           error
			resourceMap   map[string]*types.ResourceInfo
			resourceInfo  *types.ResourceInfo
			resourceAnnot = "intel.com/sriov_net_A"
		)

		It("should get a Checkpoint instance from file", func() {
			cp, err = getCheckpoint(fakeTempFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return a ResourceMap instance", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakePod",
					Namespace: "podNamespace",
					UID:       podUID,
				},
			}
			rmap, err := cp.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(rmap).NotTo(BeEmpty())
			resourceMap = rmap
		})

		It("resourceMap should have value for \"intel.com/sriov_net_A\"", func() {
			rInfo, ok := resourceMap[resourceAnnot]
			Expect(ok).To(BeTrue())
			resourceInfo = rInfo
		})

		It("should have 2 deviceIDs", func() {
			Expect(len(resourceInfo.DeviceIDs)).To(BeEquivalentTo(2))
		})

		It("should have \"0000:03:02.3\" in deviceIDs[0]", func() {
			Expect(resourceInfo.DeviceIDs[0]).To(BeEquivalentTo("0000:03:02.3"))
		})

		It("should have \"0000:03:02.0\" in deviceIDs[1]", func() {
			Expect(resourceInfo.DeviceIDs[1]).To(BeEquivalentTo("0000:03:02.0"))
		})
	})

	Context("Using faulty or incompatible information", func() {
		var (
			cp  types.ResourceClient
			err error
		)

		It("should not get a Checkpoint instance from file given bad filepath", func() {
			_, err = getCheckpoint("invalid/file/path")
			Expect(err).To(HaveOccurred())
		})

		It("should not get a Checkpoint instance from file given bad json", func() {
			sampleData := `{
				"Data": {
					"PodDeviceEntries": [
					{
						"PodUID": "970a395d-bb3b-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr1",
						"ResourceName": "intel.com/sriov_net_A",
						"DeviceIDs": [
						"0000:03:02.3",
						"0000:03:02.0"
						],
						"AllocResp": "CikKC3NyaW92X25ldF9BEhogMDAwMDowMzowMi4zIDAwMDA6MDM6MDIuMA=="
					}
					],
					"RegisteredDevices": {
					"intel.com/sriov_net_A": [
						"0000:03:02.1",
						"0000:03:02.2",
						"0000:03:02.3",
						"0000:03:02.0"
					],
					"intel.com/sriov_net_B": [
						"0000:03:06.3",
						"0000:03:06.0",
						"0000:03:06.1",
						"0000:03:06.2"
					]
					}
				},
				"Checksum": 229855270
				}`

			//missing a close bracket
			badSampleData := `BAD BAD DATA`

			fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
			fakeCheckpoint.WriteToFile([]byte(badSampleData))
			_, err = getCheckpoint(fakeTempFile)
			Expect(err).To(HaveOccurred())
			fakeCheckpoint.WriteToFile([]byte(sampleData))
		})

		It("should not return a ResourceMap instance", func() {
			cp, err = getCheckpoint(fakeTempFile)
			podUID := k8sTypes.UID("")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakePod",
					Namespace: "podNamespace",
					UID:       podUID,
				},
			}
			fmt.Println("fakePod-podID: ", fakePod.UID)
			rmap, err := cp.GetPodResourceMap(fakePod)
			Expect(err).To(HaveOccurred())
			Expect(rmap).To(BeEmpty())
		})
	})
})

var _ = AfterSuite(func() {
	fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
	err := fakeCheckpoint.DeleteFile()
	Expect(err).NotTo(HaveOccurred())
})
