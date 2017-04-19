package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"code.cloudfoundry.org/go-db-helpers/db"
)

type Config struct {
	UnderlayIP            string    `json:"underlay_ip"`
	SubnetPrefixLength    int       `json:"subnet_prefix_length"`
	Database              db.Config `json:"database"`
	HealthCheckPort       uint16    `json:"health_check_port"`
	VTEPName              string    `json:"vtep_name"`
	ConnectivityServerURL string    `json:"connectivity_server_url"`
	ServerCACertFile      string    `json:"ca_cert_file" validate:"nonzero"`
	ClientCertFile        string    `json:"client_cert_file" validate:"nonzero"`
	ClientKeyFile         string    `json:"client_key_file" validate:"nonzero"`
	VNI                   int       `json:"vni"`
	PollInterval          int       `json:"poll_interval"`
	DebugServerPort       int       `json:"debug_server_port"`
	Datastore             string    `json:"datastore"`
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
