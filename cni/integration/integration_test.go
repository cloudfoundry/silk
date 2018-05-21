package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/vishvananda/netlink"
)

const cmdTimeout = 15 * time.Second

var (
	fakeServer *gexec.Session

	cniEnv          map[string]string
	containerNS     ns.NetNS
	fakeHostNS      ns.NetNS
	cniStdin        string
	dataDir         string
	flannelSubnet   *net.IPNet
	fullNetwork     *net.IPNet
	subnetEnvFile   string
	datastorePath   string
	fakeHostNSName  string
	containerNSName string
	containerID     string
	daemonPort      int
)

var _ = BeforeEach(func() {
	By("setting up namespaces for the 'host' and 'container'")
	containerNSName = fmt.Sprintf("container-%03d", GinkgoParallelNode())
	mustSucceed("ip", "netns", "add", containerNSName)
	var err error
	containerNS, err = ns.GetNS(fmt.Sprintf("/var/run/netns/%s", containerNSName))
	Expect(err).NotTo(HaveOccurred())

	fakeHostNSName = fmt.Sprintf("host-%03d", GinkgoParallelNode())
	mustSucceed("ip", "netns", "add", fakeHostNSName)
	fakeHostNS, err = ns.GetNS(fmt.Sprintf("/var/run/netns/%s", fakeHostNSName))
	Expect(err).NotTo(HaveOccurred())

	containerID = fmt.Sprintf("test-%03d-%x", GinkgoParallelNode(), rand.Int31())

	By("setting up CNI config")
	cniEnv = map[string]string{
		"CNI_IFNAME":      "eth0",
		"CNI_CONTAINERID": containerID,
		"CNI_PATH":        paths.CNIPath,
	}
	cniEnv["CNI_NETNS"] = containerNS.Path()

	dataDir, err = ioutil.TempDir("", "cni-data-dir-")
	Expect(err).NotTo(HaveOccurred())

	flannelSubnetBaseIP, flannelSubnetCIDR, _ := net.ParseCIDR("10.255.30.0/24")
	_, fullNetwork, _ = net.ParseCIDR("10.255.0.0/16")
	flannelSubnet = &net.IPNet{
		IP:   flannelSubnetBaseIP,
		Mask: flannelSubnetCIDR.Mask,
	}

	daemonPort = 40000 + GinkgoParallelNode()
	fakeServer = startFakeDaemonInHost(daemonPort, http.StatusOK, `{"overlay_subnet": "10.255.30.0/24", "mtu": 1472}`)

	cniStdin = cniConfig(dataDir, datastorePath, daemonPort)

	datastoreDir, err := ioutil.TempDir("", "metadata-dir-")
	Expect(err).NotTo(HaveOccurred())
	datastorePath = filepath.Join(datastoreDir, "container-metadata.json")
})

var _ = AfterEach(func() {
	if fakeServer != nil {
		fakeServer.Interrupt()
		Eventually(fakeServer, "5s").Should(gexec.Exit())
	}

	containerNS.Close()
	fakeHostNS.Close()
	mustSucceed("ip", "netns", "del", fakeHostNSName)
	mustSucceed("ip", "netns", "del", containerNSName)
	Expect(os.RemoveAll(subnetEnvFile)).To(Succeed())
	Expect(os.RemoveAll(dataDir)).To(Succeed())
	Expect(os.RemoveAll(datastorePath)).To(Succeed())
})

var _ = Describe("Silk CNI Integration", func() {
	Describe("veth devices", func() {
		BeforeEach(func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
		})

		It("returns the expected CNI result", func() {
			By("calling ADD")
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			result := cniResultForCurrentVersion(sess.Out.Contents())

			inHost := ifacesWithNS(result.Interfaces, "")

			expectedCNIStdout := fmt.Sprintf(`
			{
				"cniVersion": "0.3.1",
				"interfaces": [
						{
								"name": "%s",
								"mac": "aa:aa:0a:ff:1e:02"
						},
						{
								"name": "eth0",
								"mac": "ee:ee:0a:ff:1e:02",
								"sandbox": "%s"
						}
				],
				"ips": [
						{
								"version": "4",
								"address": "10.255.30.2/32",
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
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result := cniResultForCurrentVersion(sess.Out.Contents())
			Expect(result.Interfaces).To(HaveLen(2))

			inHost := ifacesWithNS(result.Interfaces, "")
			inContainer := ifacesWithNS(result.Interfaces, containerNS.Path())

			Expect(inHost).To(HaveLen(1))
			Expect(inContainer).To(HaveLen(1))

			By("checking the link was created in the host")
			mustSucceedInFakeHost("ip", "link", "list", "dev", inHost[0].Name)

			By("checking the link was created in the container")
			mustSucceedInContainer("ip", "link", "list", "dev", inContainer[0].Name)

			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the link was deleted from the host")
			mustFailInHost("does not exist", "ip", "link", "list", "dev", inHost[0].Name)

			By("checking the link was deleted from the host")
			mustFailInContainer("does not exist", "ip", "link", "list", "dev", inContainer[0].Name)
		})

		It("can be deleted multiple times without an error status", func() {
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
		})

		It("can be deleted when silk daemon is not running", func() {
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			Expect(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.2")).To(BeAnExistingFile())
			fakeServer.Interrupt()
			Eventually(fakeServer, "5s").Should(gexec.Exit())

			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking that the ip reserved is freed")
			Expect(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.2")).NotTo(BeAnExistingFile())
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
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the host side")
			err := fakeHostNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				hostLink := hostLinkFromResult(sess.Out.Contents())

				hostAddrs, err := netlink.AddrList(hostLink, netlink.FAMILY_ALL)
				Expect(err).NotTo(HaveOccurred())

				Expect(hostAddrs).To(HaveLen(1))
				Expect(hostAddrs[0].IPNet.String()).To(Equal("169.254.0.1/32"))
				Expect(hostAddrs[0].Scope).To(Equal(int(netlink.SCOPE_LINK)))
				Expect(hostAddrs[0].Peer.String()).To(Equal("10.255.30.2/32"))
				Expect(hostLink.Attrs().HardwareAddr.String()).To(Equal("aa:aa:0a:ff:1e:02"))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the container side")
			err = containerNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				link, err := netlink.LinkByName("eth0")
				Expect(err).NotTo(HaveOccurred())

				Expect(link.Attrs().Name).To(Equal("eth0"))

				containerAddrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)

				Expect(err).NotTo(HaveOccurred())
				Expect(containerAddrs).To(HaveLen(1))
				Expect(containerAddrs[0].IPNet.String()).To(Equal("10.255.30.2/32"))
				Expect(containerAddrs[0].Scope).To(Equal(int(netlink.SCOPE_LINK)))
				Expect(containerAddrs[0].Peer.String()).To(Equal("169.254.0.1/32"))
				Expect(link.Attrs().HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:02"))
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
		})

		It("enables connectivity between the host and container", func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)

			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("enabling connectivity from the host to the container")
			// This does *not* fail as expected on Docker, but
			// does properly fail in Concourse (Garden).
			// see: https://github.com/docker/for-mac/issues/57
			mustSucceedInFakeHost("ping", "-c", "1", "10.255.30.2")

			By("enabling connectivity from the container to the host")
			mustSucceedInContainer("ping", "-c", "1", "169.254.0.1")
		})

		It("turns off ARP for veth devices", func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)

			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking the host side")
			err := fakeHostNS.Do(func(_ ns.NetNS) error {
				defer GinkgoRecover()

				hostLink := hostLinkFromResult(sess.Out.Contents())
				Expect(hostLink.Attrs().RawFlags & syscall.IFF_NOARP).To(Equal(uint32(syscall.IFF_NOARP)))

				neighs, err := netlink.NeighList(hostLink.Attrs().Index, netlink.FAMILY_V4)
				Expect(err).NotTo(HaveOccurred())

				Expect(neighs).To(HaveLen(1))
				Expect(neighs[0].IP.String()).To(Equal("10.255.30.2"))
				Expect(neighs[0].HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:02"))
				Expect(neighs[0].State).To(Equal(netlink.NUD_PERMANENT))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

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
				Expect(neighs[0].HardwareAddr.String()).To(Equal("aa:aa:0a:ff:1e:02"))
				Expect(neighs[0].State).To(Equal(netlink.NUD_PERMANENT))

				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

		})

		It("adds routes to the container", func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)

			sess := startCommandInHost("ADD", cniStdin)
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
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)

			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			const ipOnTheHost = "169.254.50.50"

			By("creating a endpoint on the host")
			mustSucceedInFakeHost("ip", "addr", "add", ipOnTheHost, "dev", "lo")

			By("checking that the container can reach that endpoint")
			mustSucceedInContainer("ping", "-c", "1", ipOnTheHost)
		})

		It("allows the container to reach IP addresses on the internet", func() {
			// NOTE: unlike all other tests in this suite
			// this one uses the REAL host namespace in order to
			// test proper packet forwarding to the internet
			// Because it messes with the REAL host namespace, it cannot safely run
			// concurrently with any other test that also touches the REAL host namespace
			// Avoid writing such tests if you can.
			By("starting the fake daemon")
			fakeServer = startFakeDaemonInRealHostNamespace(daemonPort, http.StatusOK, `{"overlay_subnet": "10.255.30.0/24", "mtu": 1350}`)

			By("calling CNI with ADD")
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
			sess := startCommandInRealHostNamespace("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("discovering the container IP")
			var cniResult current.Result
			Expect(json.Unmarshal(sess.Out.Contents(), &cniResult)).To(Succeed())
			sourceIP := fmt.Sprintf("%s/32", cniResult.IPs[0].Address.IP.String())

			By("installing the requisite iptables rule")
			iptablesRule := func(action string) []string {
				return []string{"-t", "nat", action, "POSTROUTING", "-s", sourceIP, "!", "-d", "10.255.0.0/16", "-j", "MASQUERADE"}
			}
			mustSucceed("iptables", iptablesRule("-A")...)

			By("attempting to reach the internet from the container")
			mustSucceedInContainer("curl", "-f", "example.com")

			By("removing the iptables rule from the host")
			mustSucceed("iptables", iptablesRule("-D")...)

			By("calling CNI with DEL to clean up")
			sess = startCommandInRealHostNamespace("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
		})

		Context("when MTU is not specified on the input", func() {
			It("sets the MTU based on the daemon network info", func() {
				By("calling ADD")
				sess := startCommandInHost("ADD", cniStdin)
				Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

				By("checking the host side")
				err := fakeHostNS.Do(func(_ ns.NetNS) error {
					defer GinkgoRecover()

					hostLink := hostLinkFromResult(sess.Out.Contents())
					Expect(hostLink.Attrs().MTU).To(Equal(1472))

					return nil
				})
				Expect(err).NotTo(HaveOccurred())

				By("checking the container side")
				err = containerNS.Do(func(_ ns.NetNS) error {
					defer GinkgoRecover()

					link, err := netlink.LinkByName("eth0")
					Expect(err).NotTo(HaveOccurred())
					Expect(link.Attrs().MTU).To(Equal(1472))
					return nil
				})

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when MTU is specified on the input", func() {
			It("sets the MTU based on the input", func() {
				By("calling ADD")
				cniStdin = fmt.Sprintf(`{
					"cniVersion": "0.3.1",
					"name": "my-silk-network",
					"type": "silk",
					"mtu": 1350,
					"dataDir": "%s",
					"daemonPort": %d,
					"datastore": "%s"
				}`, dataDir, daemonPort, datastorePath)
				sess := startCommandInHost("ADD", cniStdin)
				Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

				By("checking the host side")
				err := fakeHostNS.Do(func(_ ns.NetNS) error {
					defer GinkgoRecover()

					hostLink := hostLinkFromResult(sess.Out.Contents())
					Expect(hostLink.Attrs().MTU).To(Equal(1350))

					return nil
				})
				Expect(err).NotTo(HaveOccurred())

				By("checking the container side")
				err = containerNS.Do(func(_ ns.NetNS) error {
					defer GinkgoRecover()

					link, err := netlink.LinkByName("eth0")
					Expect(err).NotTo(HaveOccurred())
					Expect(link.Attrs().MTU).To(Equal(1350))
					return nil
				})

				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Describe("CNI version support", func() {
		It("only claims to support CNI spec version 0.3.1", func() {
			sess := startCommandInHost("VERSION", "{}")
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(MatchJSON(`{
          "cniVersion": "0.3.1",
          "supportedVersions": [ "0.3.1" ]
        }`))
		})
	})

	Describe("Lifecycle", func() {
		BeforeEach(func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
		})
		It("allocates and frees ips", func() {
			By("calling ADD")
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			result := cniResultForCurrentVersion(sess.Out.Contents())

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs).To(HaveLen(1))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(*result.IPs[0].Interface).To(Equal(1))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.2/32"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))

			By("checking that the ip is reserved for the correct container id")
			bytes, err := ioutil.ReadFile(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.2"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(Equal(containerID))

			By("calling DEL")
			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(BeEmpty())

			By("checking that the ip reserved is freed")
			Expect(filepath.Join(dataDir, "ipam/my-silk-network/10.255.30.2")).NotTo(BeAnExistingFile())
		})

		It("writes and deletes container metadata", func() {
			By("calling ADD")
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			By("checking that the container metadata is written")
			containerMetadata, err := ioutil.ReadFile(datastorePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(containerMetadata)).To(MatchJSON(fmt.Sprintf(`{
				"%s": {
					"handle":"%s",
					"ip":"10.255.30.2",
					"metadata":null
				}
			}`, containerNSName, containerNSName)))

			By("calling DEL")
			sess = startCommandInHost("DEL", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(BeEmpty())

			By("checking that the container metadata is deleted")
			containerMetadata, err = ioutil.ReadFile(datastorePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(containerMetadata)).NotTo(ContainSubstring("169.254.0.1"))
		})
	})

	Describe("Reserve all IPs", func() {
		var (
			containerNSList  []ns.NetNS
			numIPAllocations int
		)
		BeforeEach(func() {
			cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
			prefixSize := 29
			fakeServer = startFakeDaemonInHost(daemonPort, http.StatusOK, fmt.Sprintf(`{"overlay_subnet": "10.255.30.0/%d", "mtu": 1350}`, prefixSize))
			numIPAllocations = int(math.Pow(2, float64(32-prefixSize)) - 2)

			for i := 0; i < numIPAllocations; i++ {
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
			for i := 0; i < numIPAllocations-1; i++ {
				cniEnv["CNI_NETNS"] = containerNSList[i].Path()
				sess := startCommandInHost("ADD", cniStdin)
				Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

				result := cniResultForCurrentVersion(sess.Out.Contents())

				Expect(result.IPs).To(HaveLen(1))
				Expect(result.IPs[0].Version).To(Equal("4"))
				Expect(*result.IPs[0].Interface).To(Equal(1))
				Expect(result.IPs[0].Address.String()).To(Equal(fmt.Sprintf("10.255.30.%d/32", i+2)))
				Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))
			}

			cniEnv["CNI_NETNS"] = containerNSList[numIPAllocations-1].Path()
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(1))
			Expect(sess.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "run ipam plugin",
				"details": "failed to allocate for range 0: no IP addresses available in range set: 10.255.30.1-10.255.30.6"
				}`))
		})
	})

	Describe("when configured to use the subnet.env file", func() {
		BeforeEach(func() {
			subnetFile := writeSubnetEnvFile(flannelSubnet.String(), fullNetwork.String())
			cniStdin = cniConfigWithSubnetEnv(dataDir, datastorePath, subnetFile)
		})

		It("returns the expected CNI result", func() {
			By("calling ADD")
			sess := startCommandInHost("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			result := cniResultForCurrentVersion(sess.Out.Contents())

			inHost := ifacesWithNS(result.Interfaces, "")

			expectedCNIStdout := fmt.Sprintf(`
			{
				"cniVersion": "0.3.1",
				"interfaces": [
						{
								"name": "%s",
								"mac": "aa:aa:0a:ff:1e:02"
						},
						{
								"name": "eth0",
								"mac": "ee:ee:0a:ff:1e:02",
								"sandbox": "%s"
						}
				],
				"ips": [
						{
								"version": "4",
								"address": "10.255.30.2/32",
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
	})
})

func writeSubnetEnvFile(subnet, fullNetwork string) string {
	tempFile, err := ioutil.TempFile("", "subnet.env")
	defer tempFile.Close()
	Expect(err).NotTo(HaveOccurred())
	_, err = fmt.Fprintf(tempFile, `
FLANNEL_SUBNET=%s
FLANNEL_NETWORK=%s
FLANNEL_MTU=1472
FLANNEL_IPMASQ=false  # we'll ignore this field
`, subnet, fullNetwork)
	Expect(err).NotTo(HaveOccurred())
	return tempFile.Name()
}

func cniConfigWithExtras(dataDir, datastore string, daemonPort int, extras map[string]interface{}) string {
	conf := map[string]interface{}{
		"cniVersion": "0.3.1",
		"name":       "my-silk-network",
		"type":       "silk",
		"dataDir":    dataDir,
		"daemonPort": daemonPort,
		"datastore":  datastore,
	}
	for k, v := range extras {
		conf[k] = v
	}
	confBytes, _ := json.Marshal(conf)
	return string(confBytes)
}

func cniConfig(dataDir, datastore string, daemonPort int) string {
	return cniConfigWithExtras(dataDir, datastore, daemonPort, nil)
}

func cniConfigWithSubnetEnv(dataDir, datastore, subnetFile string) string {
	return fmt.Sprintf(`{
	"cniVersion": "0.3.1",
	"name": "my-silk-network",
	"type": "silk",
	"dataDir": "%s",
	"subnetFile": "%s",
	"datastore": "%s"
}`, dataDir, subnetFile, datastore)
}

func startCommandInHost(cniCommand, cniStdin string) *gexec.Session {
	cmd := exec.Command("ip", "netns", "exec", fakeHostNSName, paths.PathToPlugin)
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

func startFakeDaemonInHost(port, statusCode int, response string) *gexec.Session {
	if fakeServer != nil {
		fakeServer.Interrupt()
		Eventually(fakeServer, "5s").Should(gexec.Exit())
	}

	cmd := exec.Command("ip", "netns", "exec", fakeHostNSName, "ip", "link", "set", "lo", "up")
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess).Should(gexec.Exit(0))

	cmd = exec.Command("ip", "netns", "exec", fakeHostNSName, paths.PathToFakeDaemon, fmt.Sprintf("%d", port), fmt.Sprintf("%d", statusCode), response)
	sess, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	waitUntilUp := func() error {
		cmd = exec.Command("ip", "netns", "exec", fakeHostNSName, "curl", fmt.Sprintf("http://127.0.0.1:%d", port))
		return cmd.Run()
	}

	Eventually(waitUntilUp, "5s").Should(Succeed())

	return sess
}

func startFakeDaemonInRealHostNamespace(port, statusCode int, response string) *gexec.Session {
	if fakeServer != nil {
		fakeServer.Interrupt()
		Eventually(fakeServer, "5s").Should(gexec.Exit())
	}

	cmd := exec.Command(paths.PathToFakeDaemon, fmt.Sprintf("%d", port), fmt.Sprintf("%d", statusCode), response)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	waitUntilUp := func() error {
		cmd = exec.Command("curl", fmt.Sprintf("http://127.0.0.1:%d", port))
		return cmd.Run()
	}

	Eventually(waitUntilUp, "5s").Should(Succeed())

	return sess
}

func startCommandInRealHostNamespace(cniCommand, cniStdin string) *gexec.Session {
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

func mustStart(binary string, args ...string) *gexec.Session {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return sess
}

func mustSucceed(binary string, args ...string) string {
	sess := mustStart(binary, args...)
	Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}

func mustFailWith(expectedErrorSubstring string, binary string, args ...string) {
	cmd := exec.Command(binary, args...)
	allOutput, err := cmd.CombinedOutput()
	Expect(err).To(HaveOccurred())
	Expect(allOutput).To(ContainSubstring(expectedErrorSubstring))
}

func mustSucceedInContainer(binary string, args ...string) string {
	cmdArgs := []string{"netns", "exec", containerNSName, binary}
	cmdArgs = append(cmdArgs, args...)
	return mustSucceed("ip", cmdArgs...)
}

func mustStartInFakeHost(binary string, args ...string) *gexec.Session {
	cmdArgs := []string{"netns", "exec", fakeHostNSName, binary}
	cmdArgs = append(cmdArgs, args...)
	return mustStart("ip", cmdArgs...)
}

func mustSucceedInFakeHost(binary string, args ...string) string {
	cmdArgs := []string{"netns", "exec", fakeHostNSName, binary}
	cmdArgs = append(cmdArgs, args...)
	return mustSucceed("ip", cmdArgs...)
}

func mustFailInContainer(expectedErrorSubstring string, binary string, args ...string) {
	cmdArgs := []string{"netns", "exec", containerNSName, binary}
	cmdArgs = append(cmdArgs, args...)
	mustFailWith(expectedErrorSubstring, "ip", cmdArgs...)
}

func mustFailInHost(expectedErrorSubstring string, binary string, args ...string) {
	cmdArgs := []string{"netns", "exec", fakeHostNSName, binary}
	cmdArgs = append(cmdArgs, args...)
	mustFailWith(expectedErrorSubstring, "ip", cmdArgs...)
}
