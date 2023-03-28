package netinfo_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/cni/netinfo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Flannel", func() {
	var (
		filePath       string
		flannelNetInfo *netinfo.Flannel
	)

	BeforeEach(func() {
		contents := `FLANNEL_NETWORK=10.240.0.0/12
FLANNEL_SUBNET=10.255.19.1/24
FLANNEL_MTU=1450
FLANNEL_IPMASQ=false
`
		tempFile, err := ioutil.TempFile("", "subnet.env")
		Expect(err).NotTo(HaveOccurred())

		_, err = tempFile.WriteString(contents)
		Expect(err).NotTo(HaveOccurred())
		Expect(tempFile.Close()).To(Succeed())

		filePath = tempFile.Name()

		flannelNetInfo = &netinfo.Flannel{
			SubnetFilePath: filePath,
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(filePath)).To(Succeed())
	})

	It("returns the subnet and mtu in the flannel subnet env", func() {
		networkInfo, err := flannelNetInfo.Get()
		Expect(err).NotTo(HaveOccurred())
		Expect(networkInfo.OverlaySubnet).To(Equal("10.255.19.1/24"))
		Expect(networkInfo.MTU).To(Equal(1450))
	})

	Context("when there is a problem opening the file", func() {
		It("returns a helpful error", func() {
			flannelNetInfo = &netinfo.Flannel{
				SubnetFilePath: "bad-path",
			}
			_, err := flannelNetInfo.Get()
			Expect(err).To(MatchError("open bad-path: no such file or directory"))
		})
	})

	Context("when the file is malformed", func() {
		It("returns a helpful error", func() {
			Expect(ioutil.WriteFile(filePath, []byte("boo"), 0600)).To(Succeed())

			_, err := flannelNetInfo.Get()
			Expect(err).To(MatchError("unable to parse flannel subnet file"))
		})
	})

	Context("when the file doesn't have a valid subnet entry", func() {
		It("returns a helpful error", func() {
			Expect(ioutil.WriteFile(filePath, []byte(`FLANNEL_NETWORK=10.255.0.0/16
FLANNEL_SUBNET=banana
FLANNEL_MTU=1450
FLANNEL_IPMASQ=false
`), 0600)).To(Succeed())
			_, err := flannelNetInfo.Get()
			Expect(err).To(MatchError("unable to parse flannel subnet file"))
		})
	})

	Context("when the file doesn't have a valid mtu entry", func() {
		It("returns a helpful error", func() {
			Expect(ioutil.WriteFile(filePath, []byte(`FLANNEL_NETWORK=10.255.0.0/16
FLANNEL_SUBNET=10.255.19.1/24
FLANNEL_MTU=banana
FLANNEL_IPMASQ=false
`), 0600)).To(Succeed())
			_, err := flannelNetInfo.Get()
			Expect(err).To(MatchError("unable to parse MTU from subnet file"))
		})
	})
})
