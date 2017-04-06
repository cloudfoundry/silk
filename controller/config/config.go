package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Config struct {
	DebugServerPort int    `json:"debug_server_port"`
	ListenHost      string `json:"listen_host"`
	ListenPort      int    `json:"listen_port"`
	CACertFile      string `json:"ca_cert_file"`
	ServerCertFile  string `json:"server_cert_file"`
	ServerKeyFile   string `json:"server_key_file"`
}

func (c *Config) WriteToFile(configFilePath string) error {
	bytes, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %s", err)
	}
	err = ioutil.WriteFile(configFilePath, bytes, os.FileMode(0600))
	if err != nil {
		return fmt.Errorf("writing config to %s: %s", configFilePath, err)
	}
	return nil
}

func ReadFromFile(configFilePath string) (*Config, error) {
	bytes, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %s", err)
	}
	var conf Config
	err = json.Unmarshal(bytes, &conf)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling config: %s", err)
	}
	return &conf, nil
}
