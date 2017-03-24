package legacy_flannel_test

import (
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/silk/legacy_flannel"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NetworkInfo", func() {
	var (
		networkInfo legacy_flannel.NetworkInfo
		filePath    string
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

		networkInfo = legacy_flannel.NetworkInfo{}
		filePath = tempFile.Name()
	})

	AfterEach(func() {
		Expect(os.RemoveAll(filePath)).To(Succeed())
	})

	It("returns the subnet and network in the flannel subnet env", func() {
		subnet, network, err := networkInfo.DiscoverNetworkInfo(filePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(subnet).To(Equal("10.255.19.1/24"))
		Expect(network).To(Equal("10.240.0.0/12"))
	})

	Context("when there is a problem opening the file", func() {
		It("returns a helpful error", func() {
			_, _, err := networkInfo.DiscoverNetworkInfo("bad-path")
			Expect(err).To(MatchError("open bad-path: no such file or directory"))
		})
	})

	Context("when the file is malformed", func() {
		It("returns a helpful error", func() {
			Expect(ioutil.WriteFile(filePath, []byte("boo"), 0600)).To(Succeed())

			_, _, err := networkInfo.DiscoverNetworkInfo(filePath)
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
			_, _, err := networkInfo.DiscoverNetworkInfo(filePath)
			Expect(err).To(MatchError("unable to parse flannel subnet file"))
		})
	})

	Context("when the file doesn't have a valid network entry", func() {
		It("returns a helpful error", func() {
			Expect(ioutil.WriteFile(filePath, []byte(`FLANNEL_NETWORK=banana
FLANNEL_SUBNET=10.255.19.1/24
FLANNEL_MTU=1450
FLANNEL_IPMASQ=false
`), 0600)).To(Succeed())
			_, _, err := networkInfo.DiscoverNetworkInfo(filePath)
			Expect(err).To(MatchError("unable to parse flannel network from subnet file"))
		})
	})
})
