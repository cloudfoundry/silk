package acceptance_test

import (
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var cniStdIn = `
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

const cmdTimeout = 10 * time.Second

var (
	cniEnv map[string]string
)

var _ = Describe("Acceptance", func() {
	BeforeEach(func() {
		cniEnv = map[string]string{
			"CNI_IFNAME":      "eth0",
			"CNI_NETNS":       "/some/container/netns",
			"CNI_CONTAINERID": "apricot",
			"CNI_PATH":        paths.CNIPath,
		}
	})

	Describe("Lifecycle", func() {
		It("allocates and frees ips", func() {
			By("calling ADD")
			sess := startCommand("ADD")
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
			sess = startCommand("DEL")
			Eventually(sess, cmdTimeout).Should(gexec.Exit(0))
			Expect(sess.Out.Contents()).To(BeEmpty())

			By("checking that the ip reserved is freed")
			Expect("/tmp/cni/data/my-silk-network/10.255.30.2").NotTo(BeAnExistingFile())
		})
	})
})

func startCommand(cniCommand string) *gexec.Session {
	cmd := exec.Command(paths.PathToPlugin)
	cmd.Stdin = strings.NewReader(cniStdIn)
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
