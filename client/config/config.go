package config

import "code.cloudfoundry.org/go-db-helpers/db"

type Config struct {
	UnderlayIP     string    `json:"underlay_ip"`
	SubnetRange    string    `json:"subnet_range"`
	SubnetMask     int       `json:"subnet_mask"`
	Database       db.Config `json:"database"`
	LocalStateFile string    `json:"local_state_file"`
}
