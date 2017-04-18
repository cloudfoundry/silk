package acceptance_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("errors", func() {
	Describe("errors on ADD", func() {
		Context("when the subnet file is missing", func() {
			BeforeEach(func() {
				cniStdin = cniConfig(dataDir, "/path/does/not/exist", datastorePath)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "open /path/does/not/exist: no such file or directory"
			}`))
			})
		})

		Context("when the subnet file is corrupt", func() {
			BeforeEach(func() {
				subnetEnvFile = writeSubnetEnvFile("bad-subnet", fullNetwork.String())
				cniStdin = cniConfig(dataDir, subnetEnvFile, datastorePath)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "unable to parse flannel subnet file"
			}`))
			})
		})

		Context("when the ipam plugin errors on add", func() {
			BeforeEach(func() {
				subnetEnvFile = writeSubnetEnvFile("10.255.30.0/33", fullNetwork.String())
				cniStdin = cniConfig(dataDir, subnetEnvFile, datastorePath)
			})
			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "run ipam plugin",
				"details": "invalid CIDR address: 10.255.30.0/33"
			}`))
			})
		})

		Context("when the veth manager fails to create a veth pair", func() {
			It("exits with nonzero status and prints a CNI error", func() {
				cniEnv["CNI_IFNAME"] = "some-bad-eth-name"
				cniStdin = cniConfig(dataDir, subnetEnvFile, datastorePath)
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "create config",
				"details": "IfName cannot be longer than 15 characters"
			}`))
			})
		})

		Context("when the datastore is not specified", func() {
			It("fails with nonzero status and prints a CNI error", func() {
				cniStdin = fmt.Sprintf(`{
					"cniVersion": "0.3.0",
					"name": "my-silk-network",
					"type": "silk",
					"dataDir": "%s",
					"subnetFile": "%s",
					"datastore": ""
				}`, dataDir, subnetEnvFile)
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
					"code": 100,
					"msg": "write container metadata",
					"details": "open lock: open : no such file or directory"
				}`))
			})
		})
	})

	Describe("errors on DEL", func() {
		Context("when the ipam plugin errors on del", func() {
			BeforeEach(func() {
				subnetEnvFile = writeSubnetEnvFile("10.255.30.0/33", fullNetwork.String())
				cniStdin = cniConfig(dataDir, subnetEnvFile, datastorePath)
			})

			It("exits with zero status but logs the error", func() {
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(0))

				Expect(string(session.Err.Contents())).To(ContainSubstring(`invalid CIDR address: 10.255.30.0/33`))
			})
		})

		Context("when the network namespace doesn't exist", func() {
			BeforeEach(func() {
				cniEnv["CNI_NETNS"] = "/tmp/not/there"
			})
			It("exits with zero status but logs the error", func() {
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(0))

				Expect(session.Err).To(gbytes.Say(`open-netns.*/tmp/not/there.*no such file or directory`))
			})
		})

		Context("when the interface isn't present inside the container", func() {
			It("exits with zero status, but logs the error", func() {
				cniEnv["CNI_IFNAME"] = "not-there"
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(0))
				Expect(string(session.Err.Contents())).To(ContainSubstring(`"deviceName":"not-there","message":"Link not found"`))
			})
		})

		Context("when the subnet file is missing", func() {
			BeforeEach(func() {
				cniStdin = cniConfig(dataDir, "/path/does/not/exist", datastorePath)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "open /path/does/not/exist: no such file or directory"
			}`))
			})
		})

		Context("when the subnet file is corrupt", func() {
			BeforeEach(func() {
				subnetEnvFile = writeSubnetEnvFile("bad-subnet", fullNetwork.String())
				cniStdin = cniConfig(dataDir, subnetEnvFile, datastorePath)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "unable to parse flannel subnet file"
			}`))
			})
		})

		Context("when the datastore is not specified", func() {
			It("prints a CNI error", func() {
				cniStdin = fmt.Sprintf(`{
					"cniVersion": "0.3.0",
					"name": "my-silk-network",
					"type": "silk",
					"dataDir": "%s",
					"subnetFile": "%s",
					"datastore": ""
				}`, dataDir, subnetEnvFile)
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(0))
				Expect(string(session.Err.Contents())).To(MatchRegexp(`write-container-metadata.*"open lock: open : no such file or directory"`))
			})
		})
	})
})
