package lib_test

import (
	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/lib"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DatabaseHandler", func() {
	Describe("NewDatabaseHandler", func() {
		Context("when the database cannot be opened", func() {
			It("returns an error", func() {
				config := config.DatabaseConfig{
					Type:             "postgres",
					ConnectionString: "some-connection-string",
				}

				_, err := lib.NewDatabaseHandler(config)
				Expect(err).To(MatchError("connecting to database: sql: unknown driver \"postgres\" (forgotten import?)"))
			})
		})
	})
})
