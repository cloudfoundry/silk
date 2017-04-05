package state

type SubnetLease struct {
	Subnet     string `json:"subnet"`
	UnderlayIP string `json:"underlay_ip"`
}
