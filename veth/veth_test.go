package veth_test

import (
	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Veth Manager", func() {
	var (
		hostNS      ns.NetNS
		containerNS ns.NetNS
		vethManager *veth.Manager
	)

	BeforeEach(func() {
		var err error
		hostNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())
		containerNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		vethManager = &veth.Manager{
			HostNS:      hostNS,
			ContainerNS: containerNS,
		}
	})

	AfterEach(func() {
		Expect(hostNS.Close()).To(Succeed())
		Expect(containerNS.Close()).To(Succeed())
	})

	Describe("CreatePair", func() {
		It("Creates a veth with one end in the targeted namespace", func() {
			_, _, err := vethManager.CreatePair("eth0", 1500)
			Expect(err).NotTo(HaveOccurred())

			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				link, err := netlink.LinkByName("eth0")
				Expect(err).NotTo(HaveOccurred())

				Expect(link.Attrs().Name).To(Equal("eth0"))

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns both the host and container link", func() {
			hostVeth, containerVeth, err := vethManager.CreatePair("eth0", 1500)
			Expect(err).NotTo(HaveOccurred())

			Expect(hostVeth.Attrs().Name).To(MatchRegexp(`veth.*`))
			Expect(containerVeth.Attrs().Name).To(Equal("eth0"))
		})

		Context("when creating the veth pair fails", func() {
			It("returns an error", func() {
				//create veth with eth0 in container
				_, _, err := vethManager.CreatePair("eth0", 1500)
				Expect(err).NotTo(HaveOccurred())

				//create veth with eth0 in container, should fail since eth0 already exists
				_, _, err = vethManager.CreatePair("eth0", 1500)
				Expect(err).To(MatchError(ContainSubstring("container veth name provided (eth0) already exists")))
			})
		})
	})

	Describe("Destroy", func() {
		var vethName string
		BeforeEach(func() {
			_, containerVeth, err := vethManager.CreatePair("eth0", 1500)
			Expect(err).NotTo(HaveOccurred())
			vethName = containerVeth.Attrs().Name
		})

		It("destroys the veth with the given name in the given namespace", func() {
			err := vethManager.Destroy(vethName)
			Expect(err).NotTo(HaveOccurred())

			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				_, err = netlink.LinkByName(vethName)
				Expect(err).To(MatchError(ContainSubstring("not found")))

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the interface doesn't exist", func() {
			It("returns an error", func() {
				err := vethManager.Destroy("wrong-name")
				Expect(err).To(MatchError(ContainSubstring("Link not found")))
			})
		})
	})
})
