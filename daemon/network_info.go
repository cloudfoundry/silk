package daemon

type NetworkInfo struct {
	OverlaySubnet string `json:"overlay_subnet"`
	MTU           int    `json:"mtu"`
}
