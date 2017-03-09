package acceptance_test

import (
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

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
	var cniStdin string
	BeforeEach(func() {
		cniEnv = map[string]string{
			"CNI_IFNAME":      "eth0",
			"CNI_NETNS":       "/some/container/netns",
			"CNI_CONTAINERID": "apricot",
			"CNI_PATH":        paths.CNIPath,
		}
	})

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
						"dataDir": "/tmp/cni/data"
				 }
			}
			`
		})
		It("creates a veth pair", func() {
			By("calling ADD")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))

			link, err := netlink.LinkByName("silk-veth")
			Expect(err).NotTo(HaveOccurred())
			Expect(link).To(BeAssignableToTypeOf(&netlink.Veth{}))
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
			Expect(sess.Out.Contents()).To(MatchJSON(`
			{
				"ips": [
					{
						"version": "4",
						"address": "10.255.30.2/24",
						"interface": -1,
						"gateway": "10.255.30.1"
					}
				],
				"routes": [
					{
						"dst": "0.0.0.0/0"
					}
				],
				"dns": {}
			}
			`))

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
		})
		It("fails to allocate an IP if none is available", func() {
			By("exhausting all ips")
			sess := startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(MatchJSON(`
			{
				"ips": [
					{
						"version": "4",
						"address": "10.255.40.1/30",
						"interface": -1,
						"gateway": "10.0.1.1"
					}
				],
				"routes": [
					{
						"dst": "0.0.0.0/0"
					}
				],
				"dns": {}
			}
			`))
			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(MatchJSON(`
			{
				"ips": [
					{
						"version": "4",
						"address": "10.255.40.2/30",
						"interface": -1,
						"gateway": "10.0.1.1"
					}
				],
				"routes": [
					{
						"dst": "0.0.0.0/0"
					}
				],
				"dns": {}
			}
			`))

			sess = startCommand("ADD", cniStdin)
			Eventually(sess, cmdTimeout).Should(gexec.Exit(2))
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
