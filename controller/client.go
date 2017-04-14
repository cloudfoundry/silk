package controller

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/json_client"
	"code.cloudfoundry.org/lager"
)

type NonRetriableError string

func (n NonRetriableError) Error() string {
	return fmt.Sprintf("non-retriable: %s", string(n))
}

type Client struct {
	JsonClient json_client.JsonClient
}

type Lease struct {
	UnderlayIP          string `json:"underlay_ip"`
	OverlaySubnet       string `json:"overlay_subnet"`
	OverlayHardwareAddr string `json:"overlay_hardware_addr"`
}

type AcquireLeaseRequest struct {
	UnderlayIP string `json:"underlay_ip"`
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

func (c *Client) AcquireSubnetLease(underlayIP string) (Lease, error) {
	var response Lease
	request := AcquireLeaseRequest{
		UnderlayIP: underlayIP,
	}
	err := c.JsonClient.Do("PUT", "/leases/acquire", request, &response, "")
	if err != nil {
		return Lease{}, err
	}
	return response, nil
}

func (c *Client) RenewSubnetLease(lease Lease) error {
	err := c.JsonClient.Do("PUT", "/leases/renew", lease, nil, "")
	if err != nil {
		httpResponseErr, ok := err.(*json_client.HttpResponseCodeError)
		if ok && httpResponseErr.StatusCode == http.StatusConflict {
			return NonRetriableError(httpResponseErr.Message)
		}
	}
	return err
}

func (c *Client) ReleaseSubnetLease(lease Lease) error {
	err := c.JsonClient.Do("PUT", "/leases/release", lease, nil, "")
	return err
}
