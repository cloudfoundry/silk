package config

type Config struct {
	Database DatabaseConfig `json:"database"`
}

type DatabaseConfig struct {
	Type             string `json:"type"`
	ConnectionString string `json:"connection_string"`
}
