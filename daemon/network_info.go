package daemon

type NetworkInfo struct {
	OverlaySubnet string `json:"overlay_subnet"`
	MTU           uint   `json:"mtu"`
}
