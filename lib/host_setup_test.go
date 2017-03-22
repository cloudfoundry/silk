package lib_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/cloudfoundry-incubator/silk/lib"
	"github.com/cloudfoundry-incubator/silk/lib/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Host Setup", func() {

	var (
		hostNS             *fakes.NetNS
		cfg                *config.Config
		fakeLinkOperations *fakes.LinkOperations
		fakeCommon         *fakes.Common
		hostSetup          *lib.Host
		containerAddr      config.DualAddress
		hostAddr           config.DualAddress
	)

	BeforeEach(func() {
		fakeLinkOperations = &fakes.LinkOperations{}
		fakeCommon = &fakes.Common{}
		hostNS = &fakes.NetNS{}
		hostNS.DoStub = lib.NetNsDoStub

		containerAddr = config.DualAddress{IP: net.IP{10, 255, 30, 4}}
		hostAddr = config.DualAddress{IP: net.IP{169, 254, 0, 1}}

		cfg = &config.Config{}
		cfg.Host.DeviceName = "someHostDeviceName"
		cfg.Host.Namespace = hostNS
		cfg.Container.Address = containerAddr
		cfg.Host.Address = hostAddr

		hostSetup = &lib.Host{
			Common: fakeCommon,
		}
	})

	Describe("Setup", func() {
		It("calls basic setup in the host namespace", func() {
			err := hostSetup.Setup(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCommon.BasicSetupCallCount()).To(Equal(1))
			device, local, peer := fakeCommon.BasicSetupArgsForCall(0)
			Expect(device).To(Equal("someHostDeviceName"))
			Expect(local).To(Equal(hostAddr))
			Expect(peer).To(Equal(containerAddr))
		})

		Context("when the basic device setup fails", func() {
			BeforeEach(func() {
				fakeCommon.BasicSetupReturns(errors.New("beans"))
			})
			It("returns a meaningul error", func() {
				err := hostSetup.Setup(cfg)
				Expect(err).To(MatchError("setting up device in host: beans"))
			})
		})
	})
})
