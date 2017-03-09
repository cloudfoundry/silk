package veth_test

import (
	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Veth", func() {
	var (
		hostNS      ns.NetNS
		containerNS ns.NetNS
		creator     *veth.Creator
	)

	BeforeEach(func() {
		var err error
		hostNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		creator = &veth.Creator{}
	})

	AfterEach(func() {
		Expect(hostNS.Close()).To(Succeed())
		Expect(containerNS.Close()).To(Succeed())
	})

	It("Creates a veth with one end in the targeted namespace", func() {
		var err error
		containerNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		err = containerNS.Do(func(_ ns.NetNS) error {
			defer GinkgoRecover()

			err := creator.Pair("eth0", 1500, hostNS.Path())
			Expect(err).NotTo(HaveOccurred())

			return nil
		})

		err = containerNS.Do(func(_ ns.NetNS) error {
			defer GinkgoRecover()

			link, err := netlink.LinkByName("eth0")
			Expect(err).NotTo(HaveOccurred())

			Expect(link.Attrs().Name).To(Equal("eth0"))

			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
