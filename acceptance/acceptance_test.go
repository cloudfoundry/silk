package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/vishvananda/netlink"
)

const cmdTimeout = 10 * time.Second

var (
	cniEnv        map[string]string
	containerNS   ns.NetNS
	cniStdin      string
	dataDir       string
	flannelSubnet *net.IPNet
	fullNetwork   *net.IPNet
	subnetEnvFile string
)

var _ = Describe("Acceptance", func() {
	BeforeEach(func() {
		cniEnv = map[string]string{
			"CNI_IFNAME":      "eth0",
			"CNI_CONTAINERID": "apricot",
			"CNI_PATH":        paths.CNIPath,
		}

		var err error
		containerNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		cniEnv["CNI_NETNS"] = containerNS.Path()

		dataDir, err = ioutil.TempDir("", "cni-data-dir-")
		Expect(err).NotTo(HaveOccurred())

		flannelSubnetBaseIP, flannelSubnetCIDR, _ := net.ParseCIDR("10.255.30.0/24")
		_, fullNetwork, _ = net.ParseCIDR("10.255.0.0/16")
		flannelSubnet = &net.IPNet{
			IP:   flannelSubnetBaseIP,
			Mask: flannelSubnetCIDR.Mask,
		}
		subnetEnvFile = writeSubnetEnvFile(flannelSubnet.String(), fullNetwork.String())
	})

	AfterEach(func() {
		containerNS.Close() // don't bother checking errors here
		mustSucceed("iptables", "-t", "nat", "-F")
		Expect(os.RemoveAll(subnetEnvFile)).To(Succeed())
		Expect(os.RemoveAll(dataDir)).To(Succeed())
	})

	Describe("veth devices", func() {
		BeforeEach(func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)
		})

		It("returns the expected CNI result", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			result := cniResultForCurrentVersion(sess.Out.Contents())

			inHost := ifacesWithNS(result.Interfaces, "")

			expectedCNIStdout := fmt.Sprintf(`
			{
				"interfaces": [
						{
								"name": "%s",
								"mac": "aa:aa:0a:ff:1e:01"
						},
						{
								"name": "eth0",
								"mac": "ee:ee:0a:ff:1e:01",
								"sandbox": "%s"
						}
				],
				"ips": [
						{
								"version": "4",
								"address": "10.255.30.1/32",
								"gateway": "169.254.0.1",
								"interface": 1
						}
				],
				"routes": [{"dst": "0.0.0.0/0", "gw": "169.254.0.1"}],
				"dns": {}
			}
			`, inHost[0].Name, containerNS.Path())

			Expect(sess.Out.Contents()).To(MatchJSON(expectedCNIStdout))
		})

		It("creates and destroys a veth pair", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result := cniResultForCurrentVersion(sess.Out.Contents())
			Expect(result.Interfaces).To(HaveLen(2))

			inHost := ifacesWithNS(result.Interfaces, "")
			inContainer := ifacesWithNS(result.Interfaces, containerNS.Path())

			Expect(inHost).To(HaveLen(1))
			Expect(inContainer).To(HaveLen(1))

			link, err := netlink.LinkByName(inHost[0].Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().Name).To(Equal(inHost[0].Name))

			sess = startCommand("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			link, err = netlink.LinkByName(inHost[0].Name)
			Expect(err).To(MatchError(ContainSubstring("not found")))
		})

		hostLinkFromResult := func(cniResult []byte) netlink.Link {
			result := cniResultForCurrentVersion(cniResult)
			Expect(result.Interfaces).To(HaveLen(2))
			inHost := ifacesWithNS(result.Interfaces, "")
			link, err := netlink.LinkByName(inHost[0].Name)
			Expect(err).NotTo(HaveOccurred())
			return link
		}

		It("sets up the IP address and MAC address", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the host side")
			hostLink := hostLinkFromResult(sess.Out.Contents())

			hostAddrs, err := netlink.AddrList(hostLink, netlink.FAMILY_ALL)
			Expect(err).NotTo(HaveOccurred())
			Expect(hostAddrs).To(HaveLen(1))
			Expect(hostAddrs[0].IPNet.String()).To(Equal("169.254.0.1/32"))
			Expect(hostAddrs[0].Scope).To(Equal(int(netlink.SCOPE_LINK)))
			Expect(hostAddrs[0].Peer.String()).To(Equal("10.255.30.1/32"))
			Expect(hostLink.Attrs().HardwareAddr.String()).To(Equal("aa:aa:0a:ff:1e:01"))

			By("checking the container side")
			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				link, err := netlink.LinkByName("eth0")
				Expect(err).NotTo(HaveOccurred())

				Expect(link.Attrs().Name).To(Equal("eth0"))

				containerAddrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)

				Expect(err).NotTo(HaveOccurred())
				Expect(containerAddrs).To(HaveLen(1))
				Expect(containerAddrs[0].IPNet.String()).To(Equal("10.255.30.1/32"))
				Expect(containerAddrs[0].Scope).To(Equal(int(netlink.SCOPE_LINK)))
				Expect(containerAddrs[0].Peer.String()).To(Equal("169.254.0.1/32"))
				Expect(link.Attrs().HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:01"))
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
		})

		It("enables connectivity between the host and container", func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)

			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("enabling connectivity from the host to the container")
			// This does *not* fail as expected on Docker, but
			// does properly fail in Concourse (Garden).
			// see: https://github.com/docker/for-mac/issues/57
			mustSucceed("ping", "-c", "1", "10.255.30.1")

			By("enabling connectivity from the container to the host")
			mustSucceedInContainer(containerNS, "ping", "-c", "1", "169.254.0.1")
		})

		It("turns off ARP for veth devices", func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)

			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the host side")
			hostLink := hostLinkFromResult(sess.Out.Contents())
			Expect(hostLink.Attrs().RawFlags & syscall.IFF_NOARP).To(Equal(uint32(syscall.IFF_NOARP)))

			neighs, err := netlink.NeighList(hostLink.Attrs().Index, netlink.FAMILY_V4)
			Expect(err).NotTo(HaveOccurred())

			Expect(neighs).To(HaveLen(1))
			Expect(neighs[0].IP.String()).To(Equal("10.255.30.1"))
			Expect(neighs[0].HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:01"))
			Expect(neighs[0].State).To(Equal(netlink.NUD_PERMANENT))

			By("checking the container side")
			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				containerLink, err := netlink.LinkByName("eth0")
				Expect(err).NotTo(HaveOccurred())
				Expect(containerLink.Attrs().RawFlags & syscall.IFF_NOARP).To(Equal(uint32(syscall.IFF_NOARP)))

				neighs, err := netlink.NeighList(containerLink.Attrs().Index, netlink.FAMILY_V4)
				Expect(err).NotTo(HaveOccurred())

				Expect(neighs).To(HaveLen(1))
				Expect(neighs[0].IP.String()).To(Equal("169.254.0.1"))
				Expect(neighs[0].HardwareAddr.String()).To(Equal("aa:aa:0a:ff:1e:01"))
				Expect(neighs[0].State).To(Equal(netlink.NUD_PERMANENT))

				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

		})

		It("adds routes to the container", func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)

			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the routes are present inside the container")
			err := containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				link, err := netlink.LinkByName("eth0")
				Expect(err).NotTo(HaveOccurred())

				routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
				Expect(err).NotTo(HaveOccurred())

				Expect(routes).To(HaveLen(2))

				// the route returned by the IPAM result
				Expect(routes[0].Dst).To(BeNil()) // same as 0.0.0.0/0
				Expect(routes[0].Gw.String()).To(Equal("169.254.0.1"))

				// the route created when the address is assigned
				Expect(routes[1].Dst.String()).To(Equal("169.254.0.1/32"))
				Expect(routes[1].Gw).To(BeNil())

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows the container to reach IP addresses on the host namespace", func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)

			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			const ipOnTheHost = "169.254.50.50"

			By("creating a endpoint on the host")
			mustSucceed("ip", "addr", "add", ipOnTheHost, "dev", "lo")

			By("checking that the container can reach that endpoint")
			mustSucceedInContainer(containerNS, "ping", "-c", "1", ipOnTheHost)
		})

		It("allows the container to reach IP addresses on the internet", func() {
			By("running the CNI command")
			cniStdin = cniConfig(dataDir, subnetEnvFile)
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("discovering the container IP")
			var cniResult current.Result
			Expect(json.Unmarshal(sess.Out.Contents(), &cniResult)).To(Succeed())
			sourceIP := fmt.Sprintf("%s/32", cniResult.IPs[0].Address.IP.String())

			By("installing the requisite iptables rule")
			mustSucceed("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", sourceIP, "!", "-d", "10.255.0.0/16", "-j", "MASQUERADE")

			By("attempting to reach the internet from the container")
			mustSucceedInContainer(containerNS, "curl", "-f", "example.com")
		})
	})

	Describe("CNI version support", func() {
		It("only claims to support CNI spec version 0.3.0", func() {
			sess := startCommand("VERSION", "{}")
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(MatchJSON(`{
          "cniVersion": "0.3.0",
          "supportedVersions": [ "0.3.0" ]
        }`))
		})
	})

	Describe("Lifecycle", func() {
		BeforeEach(func() {
			cniStdin = cniConfig(dataDir, subnetEnvFile)
		})
		It("allocates and frees ips", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result := cniResultForCurrentVersion(sess.Out.Contents())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.1/32"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))

			By("checking that the ip is reserved for the correct container id")
			bytes, err := ioutil.ReadFile(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.1"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(Equal("apricot"))

			By("calling DEL")
			sess = startCommand("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(BeEmpty())

			By("checking that the ip reserved is freed")
			Expect(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.1")).NotTo(BeAnExistingFile())
		})
	})

	Describe("Reserve all IPs", func() {
		var (
			containerNSList []ns.NetNS
		)
		BeforeEach(func() {
			subnetEnvFile = writeSubnetEnvFile("10.255.30.0/30", fullNetwork.String())
			cniStdin = cniConfig(dataDir, subnetEnvFile)
			for i := 0; i < 3; i++ {
				containerNS, err := ns.NewNS()
				Expect(err).NotTo(HaveOccurred())
				containerNSList = append(containerNSList, containerNS)
			}
		})
		AfterEach(func() {
			for _, containerNS := range containerNSList {
				containerNS.Close()
			}
		})

		It("fails to allocate an IP if none is available", func() {
			By("exhausting all ips")
			cniEnv["CNI_NETNS"] = containerNSList[0].Path()
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result := cniResultForCurrentVersion(sess.Out.Contents())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.1/32"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))

			cniEnv["CNI_NETNS"] = containerNSList[1].Path()
			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result = cniResultForCurrentVersion(sess.Out.Contents())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Interface).To(Equal(1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.2/32"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))

			cniEnv["CNI_NETNS"] = containerNSList[2].Path()
			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(1))
			Expect(sess.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "ipam plugin failed",
				"details": "no IP addresses available in network: my-silk-network"
				}`))
		})
	})
})

func writeSubnetEnvFile(subnet, fullNetwork string) string {
	tempFile, err := ioutil.TempFile("", "subnet.env")
	defer tempFile.Close()
	Expect(err).NotTo(HaveOccurred())
	_, err = fmt.Fprintf(tempFile, `
FLANNEL_SUBNET=%s
FLANNEL_NETWORK=%s
FLANNEL_MTU=1472      # we'll handle this field in the next story
FLANNEL_IPMASQ=false  # we'll ignore this field
`, subnet, fullNetwork)
	Expect(err).NotTo(HaveOccurred())
	return tempFile.Name()
}

func cniConfig(dataDir, subnetFile string) string {
	return fmt.Sprintf(`{
	"cniVersion": "0.3.0",
	"name": "my-silk-network",
	"type": "silk",
	"dataDir": "%s",
	"subnetFile": "%s"
}`, dataDir, subnetFile)
}

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

func cniResultForCurrentVersion(output []byte) *current.Result {
	resultInterface, err := current.NewResult(output)
	Expect(err).NotTo(HaveOccurred())
	result, err := current.NewResultFromResult(resultInterface)
	Expect(err).NotTo(HaveOccurred())

	return result
}

func mustSucceed(binary string, args ...string) string {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}

func mustSucceedInContainer(containerNS ns.NetNS, binary string, args ...string) string {
	cmdArgs := []string{"netns", "exec", filepath.Base(containerNS.Path()), binary}
	cmdArgs = append(cmdArgs, args...)
	return mustSucceed("ip", cmdArgs...)
}
