package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"code.cloudfoundry.org/go-db-helpers/db"
)

type Config struct {
	UnderlayIP            string    `json:"underlay_ip"`
	SubnetRange           string    `json:"subnet_range"`
	SubnetMask            int       `json:"subnet_mask"`
	Database              db.Config `json:"database"`
	LocalStateFile        string    `json:"local_state_file"`
	HealthCheckPort       uint16    `json:"health_check_port"`
	VTEPName              string    `json:"vtep_name"`
	ConnectivityServerURL string    `json:"connectivity_server_url"`
	ServerCACertFile      string    `json:"ca_cert_file" validate:"nonzero"`
	ClientCertFile        string    `json:"client_cert_file" validate:"nonzero"`
	ClientKeyFile         string    `json:"client_key_file" validate:"nonzero"`
}

func LoadConfig(filePath string) (Config, error) {
	var cfg Config
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return cfg, fmt.Errorf("reading file %s: %s", filePath, err)
	}

	err = json.Unmarshal(contents, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("unmarshaling contents: %s", err)
	}
	return cfg, nil
}
