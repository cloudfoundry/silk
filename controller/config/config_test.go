package config_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/silk/controller/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func cloneMap(original map[string]interface{}) map[string]interface{} {
	new := map[string]interface{}{}
	for k, v := range original {
		new[k] = v
	}
	return new
}

var _ = Describe("Config.ReadFromFile", func() {
	var (
		requiredFields map[string]interface{}
	)

	BeforeEach(func() {
		requiredFields = map[string]interface{}{
			"debug_server_port":    234,
			"listen_host":          "0.0.0.0",
			"listen_port":          678,
			"ca_cert_file":         "/some/cert/file",
			"server_cert_file":     "/some/other/cert/file",
			"server_key_file":      "/some/key/file",
			"network":              "10.255.0.0/16",
			"subnet_prefix_length": 24,
			"database": db.Config{
				Type:         "mysql",
				User:         "user",
				Password:     "password",
				Host:         "127.0.0.1",
				Port:         uint16(234),
				Timeout:      1234,
				DatabaseName: "database",
			},
			"lease_expiration_seconds":    12,
			"metron_port":                 12,
			"metrics_emit_seconds":        5,
			"staleness_threshold_seconds": 5,
			"health_check_port":           999,
			"log_prefix":                  "potato",
		}
	})

	It("does not error on a valid config", func() {
		cfg := cloneMap(requiredFields)

		file, err := ioutil.TempFile(os.TempDir(), "config-")
		Expect(err).NotTo(HaveOccurred())

		Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

		_, err = config.ReadFromFile(file.Name())
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("when config file is missing a member",
		func(missingFlag, errorString string) {
			cfg := cloneMap(requiredFields)

			delete(cfg, missingFlag)

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			_, err = config.ReadFromFile(file.Name())
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("invalid config: %s", errorString))))
		},

		Entry("missing debug_server_port", "debug_server_port", "DebugServerPort: less than min"),
		Entry("missing listen_host", "listen_host", "ListenHost: zero value"),
		Entry("missing listen_port", "listen_port", "ListenPort: zero value"),
		Entry("missing ca_cert_file", "ca_cert_file", "CACertFile: zero value"),
		Entry("missing server_cert_file", "server_cert_file", "ServerCertFile: zero value"),
		Entry("missing server_key_file", "server_key_file", "ServerKeyFile: zero value"),
		Entry("missing network", "network", "Network: zero value"),
		Entry("missing subnet_prefix_length", "subnet_prefix_length", "SubnetPrefixLength: zero value"),
		Entry("missing database", "database", ""),
		Entry("missing lease_expiration_seconds", "lease_expiration_seconds", "LeaseExpirationSeconds: less than min"),
		Entry("missing metron_port", "metron_port", "MetronPort: less than min"),
		Entry("missing metrics_emit_seconds", "metrics_emit_seconds", "MetricsEmitSeconds: less than min"),
		Entry("missing staleness_threshold_seconds", "staleness_threshold_seconds", "StalenessThresholdSeconds: less than min"),
		Entry("missing health_check_port", "health_check_port", "HealthCheckPort: less than min"),
		Entry("missing log_prefix", "log_prefix", "LogPrefix: zero value"),
	)

	DescribeTable("when config file field is an invalid value",
		func(invalidField string, value interface{}, errorString string) {
			cfg := cloneMap(requiredFields)
			cfg[invalidField] = value

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			_, err = config.ReadFromFile(file.Name())
			Expect(err).To(MatchError(fmt.Sprintf("invalid config: %s", errorString)))
		},

		Entry("invalid max_open_connections", "max_open_connections", -2, "MaxOpenConnections: less than min"),
		Entry("invalid max_idle_connections", "max_idle_connections", -2, "MaxIdleConnections: less than min"),
		Entry("invalid connections_max_lifetime_seconds", "connections_max_lifetime_seconds", -2, "MaxConnectionsLifetimeSeconds: less than min"),
	)
})
