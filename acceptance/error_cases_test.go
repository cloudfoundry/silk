package acceptance_test

import (
	"io/ioutil"

	"github.com/containernetworking/cni/pkg/ns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("errors", func() {
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
	})

	Context("when the ipam plugin errors on add", func() {
		It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
			cniStdin = cniConfig("10.255.30.0/33", dataDir)
			session := startCommand("ADD", cniStdin)
			Eventually(session, cmdTimeout).Should(gexec.Exit(1))

			Expect(session.Out.Contents()).To(MatchJSON(`{
	"code": 100,
	"msg": "ipam plugin failed",
  "details": "invalid CIDR address: 10.255.30.0/33"
}`))
		})
	})

	Context("when the veth manager fails to create a veth pair", func() {
		It("exits with nonzero status and prints a CNI error", func() {
			cniEnv["CNI_IFNAME"] = "some-bad-eth-name"
			cniStdin = cniConfig("10.255.30.0/24", dataDir)
			session := startCommand("ADD", cniStdin)
			Eventually(session, cmdTimeout).Should(gexec.Exit(1))

			Expect(session.Out.Contents()).To(MatchJSON(`{
	"code": 100,
	"msg": "creating config",
  "details": "IfName cannot be longer than 15 characters"
}`))
		})
	})

	Context("when the ipam plugin errors on del", func() {
		It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
			cniStdin = cniConfig("10.255.30.0/33", dataDir)
			session := startCommand("DEL", cniStdin)
			Eventually(session, cmdTimeout).Should(gexec.Exit(1))

			Expect(session.Out.Contents()).To(MatchJSON(`{
	"code": 100,
	"msg": "ipam plugin failed",
  "details": "invalid CIDR address: 10.255.30.0/33"
}`))
		})
	})

	Context("when the veth manager fails to destroy a veth pair", func() {
		It("exits with nonzero status and prints a CNI error", func() {
			cniEnv["CNI_IFNAME"] = "some-bad-eth-name"
			cniStdin = cniConfig("10.255.30.0/24", dataDir)
			session := startCommand("DEL", cniStdin)
			Eventually(session, cmdTimeout).Should(gexec.Exit(1))

			Expect(session.Out.Contents()).To(MatchJSON(`{
	"code": 100,
	"msg": "deletion of veth pair failed",
  "details": "Deleting link: failed to lookup \"some-bad-eth-name\": numerical result out of range"
}`))
		})
	})
})
