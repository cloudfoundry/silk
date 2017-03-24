package config

type Config struct {
	UnderlayIP  string         `json:"underlay_ip"`
	SubnetRange string         `json:"subnet_range"`
	SubnetMask  string         `json:"subnet_mask"`
	Database    DatabaseConfig `json:"database"`
}

type DatabaseConfig struct {
	Type             string `json:"type"`
	ConnectionString string `json:"connection_string"`
}
