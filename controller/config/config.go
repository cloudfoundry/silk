package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"gopkg.in/validator.v2"
)

type Config struct {
	DebugServerPort           int       `json:"debug_server_port" validate:"min=1"`
	ListenHost                string    `json:"listen_host" validate:"nonzero"`
	ListenPort                int       `json:"listen_port" validate:"nonzero"`
	CACertFile                string    `json:"ca_cert_file" validate:"nonzero"`
	ServerCertFile            string    `json:"server_cert_file" validate:"nonzero"`
	ServerKeyFile             string    `json:"server_key_file" validate:"nonzero"`
	Network                   string    `json:"network" validate:"nonzero"`
	SubnetPrefixLength        int       `json:"subnet_prefix_length" validate:"nonzero"`
	Database                  db.Config `json:"database" validate:"nonzero"`
	LeaseExpirationSeconds    int       `json:"lease_expiration_seconds" validate:"min=1"`
	MetronPort                int       `json:"metron_port" validate:"min=1"`
	HealthCheckPort           int       `json:"health_check_port" validate:"min=1"`
	MetricsEmitSeconds        int       `json:"metrics_emit_seconds" validate:"min=1"`
	StalenessThresholdSeconds int       `json:"staleness_threshold_seconds" validate:"min=1"`
	LogPrefix                 string    `json:"log_prefix" validate:"nonzero"`
	MaxIdleConnections        int       `json:"max_idle_connections" validate:"min=0"`
	MaxOpenConnections        int       `json:"max_open_connections" validate:"min=0"`
}

func (c *Config) WriteToFile(configFilePath string) error {
	bytes, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %s", err)
	}
	err = ioutil.WriteFile(configFilePath, bytes, os.FileMode(0600))
	if err != nil {
		return fmt.Errorf("write config to %s: %s", configFilePath, err)
	}
	return nil
}

func ReadFromFile(configFilePath string) (*Config, error) {
	bytes, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("read config: %s", err)
	}
	var conf Config
	err = json.Unmarshal(bytes, &conf)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config: %s", err)
	}
	if err := validator.Validate(conf); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}
	return &conf, nil
}
