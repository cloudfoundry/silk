package state

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type SubnetLease struct {
	Subnet     string `json:"subnet"`
	UnderlayIP string `json:"underlay_ip"`
}

func LoadSubnetLease(filePath string) (SubnetLease, error) {
	var sl SubnetLease
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return sl, fmt.Errorf("reading file %s: %s", filePath, err)
	}

	err = json.Unmarshal(contents, &sl)
	if err != nil {
		return sl, fmt.Errorf("unmarshaling contents: %s", err)
	}
	return sl, nil
}
