package config_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/silk/controller/config"
	. "github.com/onsi/ginkgo"
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
			"lease_expiration_seconds": 12,
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

	It("errors if a required field is not set", func() {
		for fieldName, _ := range requiredFields {
			cfg := cloneMap(requiredFields)
			delete(cfg, fieldName)

			file, err := ioutil.TempFile(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			Expect(json.NewEncoder(file).Encode(cfg)).To(Succeed())

			By(fmt.Sprintf("checking that %s is required", fieldName))
			_, err = config.ReadFromFile(file.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("invalid config:"))
		}
	})
})
