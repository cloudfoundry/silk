package config_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/client/config"
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

var _ = Describe("Config.LoadConfig", func() {
	var (
		requiredFields map[string]interface{}
	)

	BeforeEach(func() {
		requiredFields = map[string]interface{}{
			"underlay_ip":                 "1.2.3.4",
			"subnet_prefix_length":        24,
			"overlay_network":             "10.255.0.0/16",
			"health_check_port":           22222,
			"vtep_name":                   "silk-vxlan",
			"connectivity_server_url":     "https://silk-controller.something",
			"ca_cert_file":                "/some/cert/file.pem",
			"client_cert_file":            "/some/client-cert/file.pem",
			"client_key_file":             "/some/client-key/file.pem",
			"vni":                         44,
			"poll_interval":               5,
			"debug_server_port":           22229,
			"datastore":                   "/some/data-store-file.json",
			"partition_tolerance_seconds": 25,
			"client_timeout_seconds":      5,
			"metron_port":                 5435,
			"vtep_port":                   1234,
			"log_prefix":                  "some-prefix",
		}
	})

	It("does not error on a valid config", func() {
		cfg := cloneMap(requiredFields)

		file, err := ioutil.TempFile(os.TempDir(), "config-")
		Expect(err).NotTo(HaveOccurred())

		Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

		_, err = config.LoadConfig(file.Name())
		Expect(err).NotTo(HaveOccurred())
	})

	It("errors if a required field is not set", func() {
		for fieldName, _ := range requiredFields {
			cfg := cloneMap(requiredFields)
			delete(cfg, fieldName)

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			By(fmt.Sprintf("checking that %s is required", fieldName))
			_, err = config.LoadConfig(file.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("invalid config:"))
		}
	})

	Context("when single ip only is specified", func() {
		It("sets SingleIPOnly", func() {
			cfg := cloneMap(requiredFields)
			cfg["single_ip_only"] = true

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			loadedConfig, err := config.LoadConfig(file.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedConfig.SingleIPOnly).To(Equal(true))
		})
	})

	Context("when vxlan_interface_name is specified", func() {
		It("sets VxlanInterfaceName", func() {
			cfg := cloneMap(requiredFields)
			cfg["vxlan_interface_name"] = "something"

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			loadedConfig, err := config.LoadConfig(file.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedConfig.VxlanInterfaceName).To(Equal("something"))
		})
	})
})
