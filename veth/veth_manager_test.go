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
		containerNS ns.NetNS
		vethManager *veth.Manager
	)

	BeforeEach(func() {
		var err error
		containerNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		vethManager, err = veth.NewManager(containerNS.Path())
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(containerNS.Close()).To(Succeed())
	})

	Describe("NewManager", func() {
		It("Creates a new manager", func() {
			currentNS, err := ns.GetCurrentNS()
			Expect(err).NotTo(HaveOccurred())

			Expect(vethManager.HostNS.Path()).To(Equal(currentNS.Path()))
			Expect(vethManager.ContainerNS.Path()).To(Equal(containerNS.Path()))
		})

		Context("When the container namespace cannot be found from its path", func() {
			It("returns an error", func() {
				_, err := veth.NewManager("some-bad-path")
				Expect(err.Error()).To(ContainSubstring("Failed to create veth manager:"))
			})
		})
	})

	Describe("CreatePair", func() {
		It("Creates a veth with one end in the targeted namespace", func() {
			_, err := vethManager.CreatePair("eth0", 1500)
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

		It("returns both the host and container link and namespaces", func() {
			vethPair, err := vethManager.CreatePair("eth0", 1500)
			Expect(err).NotTo(HaveOccurred())

			Expect(vethPair.Host.Link.Attrs().Name).To(MatchRegexp(`veth.*`))
			Expect(vethPair.Container.Link.Attrs().Name).To(Equal("eth0"))
			Expect(vethPair.Host.Namespace).To(Equal(vethManager.HostNS))
			Expect(vethPair.Container.Namespace).To(Equal(vethManager.ContainerNS))
		})

		Context("when creating the veth pair fails", func() {
			It("returns an error", func() {
				//create veth with eth0 in container
				_, err := vethManager.CreatePair("eth0", 1500)
				Expect(err).NotTo(HaveOccurred())

				//create veth with eth0 in container, should fail since eth0 already exists
				_, err = vethManager.CreatePair("eth0", 1500)
				Expect(err).To(MatchError(ContainSubstring("container veth name provided (eth0) already exists")))
			})
		})
	})

	Describe("Destroy", func() {
		var vethName string
		BeforeEach(func() {
			vethPair, err := vethManager.CreatePair("eth0", 1500)
			Expect(err).NotTo(HaveOccurred())
			vethName = vethPair.Container.Link.Attrs().Name
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
