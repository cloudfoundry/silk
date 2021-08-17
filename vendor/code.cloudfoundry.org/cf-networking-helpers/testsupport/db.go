package testsupport

import (
	"context"
	"fmt"
	"os"
	"strings"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/lager/lagertest"

	"log"
	"time"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func CreateDatabase(config db.Config) {
	config.Timeout = 120
	dbToCreate := config.DatabaseName
	config.DatabaseName = ""
	fmt.Fprintln(ginkgo.GinkgoWriter, fmt.Sprintf("%s Creating database %s", time.Now().String(), dbToCreate))
	connection := getDbConnection(config)
	defer connection.ConnectionPool.Close()
	_, err := connection.ConnectionPool.Exec(fmt.Sprintf("CREATE DATABASE %s", dbToCreate))
	Expect(err).NotTo(HaveOccurred())
}

func RemoveDatabase(config db.Config) {
	config.Timeout = 120

	dbToDrop := config.DatabaseName
	config.DatabaseName = ""

	connection := getDbConnection(config)
	defer connection.ConnectionPool.Close()
	_, err := connection.ConnectionPool.Exec(fmt.Sprintf("DROP DATABASE %s", dbToDrop))
	if err != nil {
		fmt.Fprintln(ginkgo.GinkgoWriter, fmt.Sprintf("%+v", err))
	}
}

type dbConnection struct {
	ConnectionPool *db.ConnWrapper
	Err            error
}

func getDbConnection(conf db.Config) dbConnection {
	retriableConnector := db.RetriableConnector{
		Logger:        lagertest.NewTestLogger("test"),
		Connector:     db.GetConnectionPool,
		Sleeper:       nil,
		RetryInterval: 0 * time.Second,
		MaxRetries:    0,
	}

	channel := make(chan dbConnection)
	go func() {
		connection, err := retriableConnector.GetConnectionPool(conf, context.Background())
		channel <- dbConnection{connection, err}
	}()
	var connectionResult dbConnection
	select {
	case connectionResult = <-channel:
	case <-time.After(5 * time.Second):
		log.Fatalf("%s.testsupport: db connection timeout", "db-helper")
	}
	if connectionResult.Err != nil {
		log.Fatalf("%s.testsupport: db connect: %s", "db-helper", connectionResult.Err)
	}
	return connectionResult
}

const DefaultDBTimeout = 5

func getPostgresDBConfig() db.Config {
	return db.Config{
		Type:     "postgres",
		User:     "postgres",
		Password: "",
		Host:     "127.0.0.1",
		Port:     5432,
		Timeout:  DefaultDBTimeout,
	}
}

func getMySQLDBConfig() db.Config {
	return db.Config{
		Type:     "mysql",
		User:     "root",
		Password: "password",
		Host:     "127.0.0.1",
		Port:     3306,
		Timeout:  DefaultDBTimeout,
	}
}

func GetDBConfig() db.Config {
	dbEnv := os.Getenv("DB")
	switch {
	case strings.HasPrefix(dbEnv, "mysql"):
		return getMySQLDBConfig()
	case strings.HasPrefix(dbEnv, "postgres"):
		return getPostgresDBConfig()
	default:
		panic("unable to determine database to use.  Set environment variable DB")
	}
}
