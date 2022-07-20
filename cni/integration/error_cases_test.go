package integration_test

import (
	"fmt"
	"net/http"

	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type cniDatabaseError struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Details string `json:"details"`
}

var _ = Describe("errors", func() {
	Describe("errors on ADD", func() {
		Context("when the subnet file is missing", func() {
			BeforeEach(func() {
				cniStdin = cniConfigWithSubnetEnv(dataDir, datastorePath, "/path/does/not/exist")
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "get netinfo: open /path/does/not/exist: no such file or directory"
			}`))
			})
		})

		Context("when the subnet file is corrupt", func() {
			BeforeEach(func() {
				subnetEnvFile = writeSubnetEnvFile("bad-subnet", fullNetwork.String())
				cniStdin = cniConfigWithSubnetEnv(dataDir, datastorePath, subnetEnvFile)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "get netinfo: unable to parse flannel subnet file"
			}`))
			})
		})

		Context("when the MTU is less than 0", func() {
			BeforeEach(func() {
				fakeServer = startFakeDaemonInHost(daemonPort, http.StatusOK, `{"overlay_subnet": "10.255.30.0/24", "mtu": 1472}`)
				cniStdin = fmt.Sprintf(`{
				"cniVersion": "0.3.1",
				"name": "my-silk-network",
				"type": "silk",
				"mtu": -123,
				"dataDir": "%s",
				"daemonPort": %d,
				"datastore": "%s"}`, dataDir, daemonPort, datastorePath)
			})
			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
        "code": 100,
				"msg": "discover network info",
        "details": "invalid config: MTU: less than min"
        }`))
			})
		})

		Context("when the daemon url fails to return a response", func() {
			BeforeEach(func() {
				if fakeServer != nil {
					fakeServer.Interrupt()
					Eventually(fakeServer, "5s").Should(gexec.Exit())
				}
				cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				cniError := &cniDatabaseError{}
				err := json.Unmarshal(session.Out.Contents(), cniError)
				Expect(err).ToNot(HaveOccurred())

				Expect(cniError.Code).To(Equal(100))
				Expect(cniError.Msg).To(Equal("discover network info"))
				expectedCNIDetails := fmt.Sprintf(`get netinfo: json client do: http client do: Get "http://127.0.0.1:%[1]d/": dial tcp 127.0.0.1:%[1]d: .* connection refused`, daemonPort)
				Expect(cniError.Details).To(MatchRegexp(expectedCNIDetails))
			})
		})

		Context("when the daemon network info cannot be unmarshaled", func() {
			BeforeEach(func() {
				fakeServer = startFakeDaemonInHost(daemonPort, http.StatusOK, `bad response`)
				cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
			})

			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "discover network info",
				"details": "get netinfo: json client do: json unmarshal: invalid character 'b' looking for beginning of value"
			}`))
			})
		})

		Context("when the ipam plugin errors on add", func() {
			BeforeEach(func() {
				fakeServer = startFakeDaemonInHost(daemonPort, http.StatusOK, `{"overlay_subnet": "10.255.30.0/33", "mtu": 1350}`)
				cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
			})
			It("exits with nonzero status and prints a CNI error result as JSON to stdout", func() {
				session := startCommandInHost("ADD", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(1))

				Expect(session.Out.Contents()).To(MatchJSON(`{
				"code": 100,
				"msg": "generate ipam config",
				"details": "invalid subnet: invalid CIDR address: 10.255.30.0/33"
			}`))
			})
		})

		Context("when the veth manager fails to create a veth pair", func() {
			It("exits with nonzero status and prints a CNI error", func() {
				cniEnv["CNI_IFNAME"] = "some-bad-eth-name"
				cniStdin = cniConfig(dataDir, datastorePath, daemonPort)
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
					"cniVersion": "0.3.1",
					"name": "my-silk-network",
					"type": "silk",
					"dataDir": "%s",
					"daemonPort": %d,
					"datastore": ""
				}`, dataDir, daemonPort)
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

		Context("when the datastore is not specified", func() {
			It("prints a CNI error", func() {
				cniStdin = fmt.Sprintf(`{
					"cniVersion": "0.3.1",
					"name": "my-silk-network",
					"type": "silk",
					"dataDir": "%s",
					"daemonPort": %d,
					"datastore": ""
				}`, dataDir, daemonPort)
				session := startCommandInHost("DEL", cniStdin)
				Eventually(session, cmdTimeout).Should(gexec.Exit(0))
				Expect(string(session.Err.Contents())).To(MatchRegexp(`delete-from-container-metadata.*"open lock: open : no such file or directory"`))
			})
		})
	})
})
