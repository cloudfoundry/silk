package veth2_test

import (
	"fmt"
	"math/rand"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/cloudfoundry-incubator/silk/veth2"
	"github.com/containernetworking/cni/pkg/ns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("VethPairCreator", func() {
	Describe("Create", func() {

		var (
			containerNS ns.NetNS
			hostNS      ns.NetNS
			cfg         config.Config
			creator     *veth2.VethPairCreator
		)

		BeforeEach(func() {
			var err error
			containerNS, err = ns.NewNS()
			Expect(err).NotTo(HaveOccurred())
			hostNS, err = ns.NewNS()
			Expect(err).NotTo(HaveOccurred())

			cfg = config.Config{}
			cfg.Container.TemporaryDeviceName = fmt.Sprintf("c-%x", rand.Int31())
			cfg.Container.Namespace = containerNS
			cfg.Container.MTU = 1234
			cfg.Host.DeviceName = fmt.Sprintf("h-%x", rand.Int31())
			cfg.Host.Namespace = hostNS

			creator = &veth2.VethPairCreator{}
		})

		AfterEach(func() {
			containerNS.Close() // don't bother checking errors here
			hostNS.Close()
		})

		It("creates a correctly-named veth device in the host namespace with the correct MTU", func() {
			Expect(creator.Create(cfg)).To(Succeed())
			err := hostNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				dev, err := netlink.LinkByName(cfg.Host.DeviceName)
				Expect(err).NotTo(HaveOccurred())

				Expect(dev.Attrs().MTU).To(Equal(1234))

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a temporarily-named veth device in the container namespace with the correct MTU", func() {
			Expect(creator.Create(cfg)).To(Succeed())
			err := containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()
				dev, err := netlink.LinkByName(cfg.Container.TemporaryDeviceName)
				Expect(err).NotTo(HaveOccurred())

				Expect(dev.Attrs().MTU).To(Equal(1234))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when adding the link fails", func() {
			BeforeEach(func() {
				cfg.Container.TemporaryDeviceName = "some-name-that-is-too-long"
			})

			It("wraps and returns the error", func() {
				err := creator.Create(cfg)
				Expect(err).To(MatchError("creating veth pair: numerical result out of range"))
			})
		})

		Context("when moving the container-side veth into the container fails", func() {
			BeforeEach(func() {
				cfg.Container.Namespace.Close()
			})

			It("wraps and returns the error", func() {
				err := creator.Create(cfg)
				Expect(err).To(MatchError("failed to move veth to container namespace: bad file descriptor"))
			})
		})
	})
})
