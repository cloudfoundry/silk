package controller

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-networking-helpers/json_client"
	"code.cloudfoundry.org/lager/v3"
)

type NonRetriableError string

func (n NonRetriableError) Error() string {
	return string(n)
}

type Client struct {
	JsonClient json_client.JsonClient
}

type Lease struct {
	UnderlayIP          string `json:"underlay_ip"`
	OverlaySubnet       string `json:"overlay_subnet"`
	OverlayHardwareAddr string `json:"overlay_hardware_addr"`
}

type ReleaseLeaseRequest struct {
	UnderlayIP string `json:"underlay_ip"`
}

type AcquireLeaseRequest struct {
	UnderlayIP      string `json:"underlay_ip"`
	SingleOverlayIP bool   `json:"single_overlay_ip"`
}

func NewClient(logger lager.Logger, httpClient json_client.HttpClient, baseURL string) *Client {
	return &Client{
		JsonClient: json_client.New(logger, httpClient, baseURL),
	}
}

func (c *Client) GetActiveLeases() ([]Lease, error) {
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
	return c.acquireLease(underlayIP, false)
}

func (c *Client) AcquireSingleOverlayIPLease(underlayIP string) (Lease, error) {
	return c.acquireLease(underlayIP, true)
}

func (c *Client) acquireLease(underlayIP string, singleOverlayIP bool) (Lease, error) {
	var response Lease
	request := AcquireLeaseRequest{
		UnderlayIP:      underlayIP,
		SingleOverlayIP: singleOverlayIP,
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
			return NonRetriableError(fmt.Sprintf("non-retriable: %s", httpResponseErr.Message))
		}
	}
	return err
}

func (c *Client) ReleaseSubnetLease(underlayIP string) error {
	request := ReleaseLeaseRequest{
		UnderlayIP: underlayIP,
	}
	err := c.JsonClient.Do("PUT", "/leases/release", request, nil, "")
	if err != nil {
		return err
	}
	return nil
}
