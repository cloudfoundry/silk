package controller

import (
	"code.cloudfoundry.org/go-db-helpers/json_client"
	"code.cloudfoundry.org/lager"
)

type Client struct {
	JsonClient json_client.JsonClient
}

type Lease struct {
	UnderlayIP    string `json:"underlay_ip"`
	OverlaySubnet string `json:"overlay_subnet"`
}

func NewClient(logger lager.Logger, httpClient json_client.HttpClient, baseURL string) *Client {
	return &Client{
		JsonClient: json_client.New(logger, httpClient, baseURL),
	}
}

func (c *Client) GetRoutableLeases() ([]Lease, error) {
	var response struct {
		Leases []Lease
	}
	err := c.JsonClient.Do("GET", "/leases", nil, &response, "")
	if err != nil {
		return nil, err
	}
	return response.Leases, nil
}
