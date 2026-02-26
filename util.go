package main

import (
	"net"
	"strings"

	"github.com/nettica-com/nettica-admin/model"
)

func Sanitize(s string) string {

	// remove path and shell special characters
	r := strings.NewReplacer(
		"../", "",
		"..\\", "",
		"/#", "",
		"..", "",
		"/", "",
		"\\", "",
		":", "",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
		"&", "",
		"%", "",
		"$", "",
		"#", "",
		"@", "",
		"!", "",
	)

	// Repeat replacement until `s` no longer changes.
	for {
		n := r.Replace(s)
		if n == s {
			return n // No change, return the result.
		}
		s = n // Update `s` for the next iteration.
	}
}

// function compares two devices and returns true if they are the same
func CompareDevices(d1 *model.Device, d2 *model.Device) bool {

	if (d1 == nil) || (d2 == nil) {
		return false
	}

	if d1.Id != d2.Id {
		return false
	}

	if d1.Registered != d2.Registered {
		return false
	}

	if d1.InstanceID != d2.InstanceID {
		return false
	}

	if d1.EZCode != d2.EZCode {
		return false
	}

	if d1.Name != d2.Name {
		return false
	}

	if d1.ApiKey != d2.ApiKey {
		return false
	}

	if d1.Server != d2.Server {
		return false
	}

	if d1.Logging != d2.Logging {
		return false
	}

	if d1.CheckInterval != d2.CheckInterval {
		return false
	}

	if d1.Enable != d2.Enable {
		return false
	}

	if d1.Platform != d2.Platform {
		return false
	}

	if d1.Version != d2.Version {
		return false
	}

	if d1.SourceAddress != d2.SourceAddress {
		return false
	}

	if d1.Updated != d2.Updated {
		return false
	}

	if d1.Created != d2.Created {
		return false
	}

	if d1.AccountID != d2.AccountID {
		return false
	}

	if d1.ServiceGroup != d2.ServiceGroup {
		return false
	}

	if d1.ServiceApiKey != d2.ServiceApiKey {
		return false
	}

	if !boolPtrEq(d1.TextEnabled, d2.TextEnabled) {
		return false
	}

	if !boolPtrEq(d1.VideoEnabled, d2.VideoEnabled) {
		return false
	}

	if !boolPtrEq(d1.ConferenceEnabled, d2.ConferenceEnabled) {
		return false
	}

	return true
}

// boolPtrEq returns true when both pointers are nil, or both are non-nil
// and point to the same bool value.
func boolPtrEq(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// function merges two devices, d1 is the source, d2 is the destination
// use this function to merge messages from the server back to the client.
func MergeDevices(d1 *model.Device, d2 *model.Device) {

	if (d1 == nil) || (d2 == nil) {
		return
	}

	// Some properties, like Quiet and Debug cannot be controlled by the server
	// InstanceID is not managed by the server
	// Version is not managed by the server

	if d1.Id != d2.Id {
		d2.Id = d1.Id
	}

	if d1.Registered {
		d2.Registered = true
	}

	if d1.Name != "" {
		d2.Name = d1.Name
	}

	if d1.ApiKey != "" {
		d2.ApiKey = d1.ApiKey
	}

	if d1.Server != "" {
		d2.Server = d1.Server
	}

	if d1.CheckInterval != 0 {
		d2.CheckInterval = d1.CheckInterval
	}
	if d2.CheckInterval == 0 {
		d2.CheckInterval = 10
	}

	d2.Enable = d1.Enable

	if d1.Platform != "" {
		d2.Platform = d1.Platform
	}

	if d1.SourceAddress != "" {
		d2.SourceAddress = d1.SourceAddress
	}

	d2.Updated = d1.Updated
	d2.Created = d1.Created

	if d1.AccountID != "" {
		d2.AccountID = d1.AccountID
	}

	if d1.ServiceGroup != "" {
		d2.ServiceGroup = d1.ServiceGroup
	}

	if d1.ServiceApiKey != "" {
		d2.ServiceApiKey = d1.ServiceApiKey
	}

	if d1.InstanceID != "" {
		d2.InstanceID = d1.InstanceID
	}

	d2.EZCode = d1.EZCode

	// Capability flags are server-authoritative: always take the server's value,
	// including nil (meaning "not set / unknown").
	d2.TextEnabled = d1.TextEnabled
	d2.VideoEnabled = d1.VideoEnabled
	d2.ConferenceEnabled = d1.ConferenceEnabled

}

// GetNetworkAddress gets the valid start of a subnet
func GetNetworkAddress(cidr string) (string, error) {

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	networkAddr := ipnet.String()

	return networkAddr, nil

}

// GetLocalSubnets gets the local subnets
// While the code is identical on all platforms, the results
// actually differ.  For example, on Windows individual IP addresses
// are returned as /32s, while on Linux they're returned as /24s
// (or I suppose the appropriate subnet mask for the network)
func GetLocalSubnets() ([]*net.IPNet, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return nil, err
	}

	subnets := make([]*net.IPNet, 0)

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				subnets = append(subnets, v)
			}
		}
	}
	return subnets, nil
}
