package veth_test

import (
	"net"

	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Veth", func() {
	Describe("Creator", func() {
		var (
			hostNS      ns.NetNS
			containerNS ns.NetNS
			creator     *veth.Creator
		)

		BeforeEach(func() {
			var err error
			hostNS, err = ns.NewNS()
			Expect(err).NotTo(HaveOccurred())
			containerNS, err = ns.NewNS()
			Expect(err).NotTo(HaveOccurred())

			creator = &veth.Creator{}
		})

		AfterEach(func() {
			Expect(hostNS.Close()).To(Succeed())
			Expect(containerNS.Close()).To(Succeed())
		})

		It("Creates a veth with one end in the targeted namespace", func() {
			_, _, err := creator.Pair("eth0", 1500, hostNS, containerNS)
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
			hostVeth, containerVeth, err := creator.Pair("eth0", 1500, hostNS, containerNS)
			Expect(err).NotTo(HaveOccurred())

			Expect(hostVeth.Attrs().Name).To(MatchRegexp(`veth.*`))
			Expect(containerVeth.Attrs().Name).To(Equal("eth0"))
		})

		Context("when creating the veth pair fails", func() {
			It("returns an error", func() {
				//create veth with eth0 in container
				_, _, err := creator.Pair("eth0", 1500, hostNS, containerNS)
				Expect(err).NotTo(HaveOccurred())

				//create veth with eth0 in container, should fail since eth0 already exists
				_, _, err = creator.Pair("eth0", 1500, hostNS, containerNS)
				Expect(err).To(MatchError(ContainSubstring("container veth name provided (eth0) already exists")))
			})
		})
	})

	Describe("Destroyer", func() {
		var (
			containerNS ns.NetNS
			destroyer   *veth.Destroyer
		)

		BeforeEach(func() {
			var err error
			containerNS, err = ns.NewNS()
			Expect(err).NotTo(HaveOccurred())

			destroyer = &veth.Destroyer{}

			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				veth := &netlink.Veth{
					LinkAttrs: netlink.LinkAttrs{
						Name:  "some-name",
						Flags: net.FlagUp,
						MTU:   1500,
					},
					PeerName: "some-peer",
				}
				if err := netlink.LinkAdd(veth); err != nil {
					return err
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(containerNS.Close()).To(Succeed())
		})

		It("destroys the veth with the given name in the given namespace", func() {
			err := destroyer.Destroy("some-name", containerNS.Path())
			Expect(err).NotTo(HaveOccurred())

			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				_, err = netlink.LinkByName("some-name")
				Expect(err).To(MatchError(ContainSubstring("not found")))

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when getting the ns fails", func() {
			It("returns an error", func() {
				err := destroyer.Destroy("some-name", "/some/bad/path")
				Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
			})
		})

		Context("when the interface doesn't exist", func() {
			It("returns an error", func() {
				err := destroyer.Destroy("wrong-name", containerNS.Path())
				Expect(err).To(MatchError(ContainSubstring("Link not found")))
			})
		})
	})
})
