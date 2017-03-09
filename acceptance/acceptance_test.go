package acceptance_test

import (
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/vishvananda/netlink"
)

const cmdTimeout = 10 * time.Second

var (
	cniEnv map[string]string
)

var _ = Describe("Acceptance", func() {
	var (
		cniStdin        string
		containerNSPath string
	)
	BeforeEach(func() {
		cniEnv = map[string]string{
			"CNI_IFNAME":      "eth0",
			"CNI_CONTAINERID": "apricot",
			"CNI_PATH":        paths.CNIPath,
		}

		containerNS, err := ns.NewNS()
		Expect(err).NotTo(HaveOccurred())
		containerNSPath = containerNS.Path()

		cniEnv["CNI_NETNS"] = containerNSPath
	})

	//TODO make this work
	Describe("veth devices", func() {
		BeforeEach(func() {
			cniStdin = `
			{
				"cniVersion": "0.3.0",
				"name": "silk-veth-test",
				"type": "silk",
				"ipam": {
						"type": "host-local",
						"subnet": "10.255.30.0/24",
						"routes": [ { "dst": "0.0.0.0/0" } ],
						"dataDir": "/tmp/cni/data-veth-test"
				 }
			}
			`
		})

		It("creates a veth pair", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			resultInterface, err := current.NewResult(sess.Out.Contents())
			Expect(err).NotTo(HaveOccurred())
			result, err := current.NewResultFromResult(resultInterface)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Interfaces).To(HaveLen(2))

			inHost := ifacesWithNS(result.Interfaces, "")
			inContainer := ifacesWithNS(result.Interfaces, containerNSPath)

			Expect(inHost).To(HaveLen(1))
			Expect(inContainer).To(HaveLen(1))

			link, err := netlink.LinkByName(inHost[0].Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().Name).To(Equal(inHost[0].Name))
		})
	})

	Describe("Lifecycle", func() {
		BeforeEach(func() {
			cniStdin = `
			{
				"cniVersion": "0.3.0",
				"name": "my-silk-network",
				"type": "silk",
				"ipam": {
						"type": "host-local",
						"subnet": "10.255.30.0/24",
						"routes": [ { "dst": "0.0.0.0/0" } ],
						"dataDir": "/tmp/cni/data"
				 }
			}
			`
		})
		It("allocates and frees ips", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			resultInterface, err := current.NewResult(sess.Out.Contents())
			Expect(err).NotTo(HaveOccurred())
			result, err := current.NewResultFromResult(resultInterface)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(-1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.2/24"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("10.255.30.1"))

			By("checking that the ip is reserved for the correct container id")
			bytes, err := ioutil.ReadFile("/tmp/cni/data/my-silk-network/10.255.30.2")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(Equal("apricot"))

			By("calling DEL")
			sess = startCommand("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(BeEmpty())

			By("checking that the ip reserved is freed")
			Expect("/tmp/cni/data/my-silk-network/10.255.30.2").NotTo(BeAnExistingFile())
		})
	})

	Describe("Reserve all IPs", func() {
		var (
			containerNSList []string
		)
		BeforeEach(func() {
			cniStdin = `
			{
				"cniVersion": "0.3.0",
				"name": "my-silk-network-exhaust",
				"type": "silk",
				"ipam": {
						"type": "host-local",
						"subnet": "10.255.40.0/30",
						"routes": [ { "dst": "0.0.0.0/0" } ],
						"gateway": "10.0.1.1",
						"dataDir": "/tmp/cni/data-exhaust"
				 }
			}
			`
			for i := 0; i < 3; i++ {
				containerNS, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				containerNSList = append(containerNSList, containerNS.Path())
			}
		})
		It("fails to allocate an IP if none is available", func() {
			By("exhausting all ips")
			cniEnv["CNI_NETNS"] = containerNSList[0]
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			resultInterface, err := current.NewResult(sess.Out.Contents())
			Expect(err).NotTo(HaveOccurred())
			result, err := current.NewResultFromResult(resultInterface)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(-1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.40.1/30"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("10.0.1.1"))

			cniEnv["CNI_NETNS"] = containerNSList[1]
			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			resultInterface, err = current.NewResult(sess.Out.Contents())
			Expect(err).NotTo(HaveOccurred())
			result, err = current.NewResultFromResult(resultInterface)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(-1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.40.2/30"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("10.0.1.1"))

			cniEnv["CNI_NETNS"] = containerNSList[2]
			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(1))
			Expect(sess.Err).To(gbytes.Say("no IP addresses available in network: my-silk-network-exhaust"))
		})
	})
})

func startCommand(cniCommand, cniStdin string) *gexec.Session {
	cmd := exec.Command(paths.PathToPlugin)
	cmd.Stdin = strings.NewReader(cniStdin)
	// Set command env
	cniEnv["CNI_COMMAND"] = cniCommand
	for k, v := range cniEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Run command
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return sess
}

func ifacesWithNS(result []*current.Interface, nsPath string) []*current.Interface {
	ret := []*current.Interface{}
	for _, iface := range result {
		if iface.Sandbox == nsPath {
			ret = append(ret, iface)
		}
	}
	return ret
}
