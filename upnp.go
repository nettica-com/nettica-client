package main

import (
	"net"
	"strings"

	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

func isBogon(ip string) bool {
	// Check to see if the ip address is a bogon
	// https://en.wikipedia.org/wiki/Bogon_filtering
	// https://www.team-cymru.org/Bogons/

	addr := net.ParseIP(ip)
	if addr == nil {
		log.Error("Invalid IP address")
		return true
	}
	// check if the address is a bogon
	if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		log.Error("IP address is a bogon")
		return true
	}
	if ip == "0.0.0.0" {
		return true
	}
	return false
}

func ConfigureUPnP(host model.Host) error {

	if host.Current.UPnP {

		log.Infof("***UPNP*** Configuring UPnP for %s", host.Name)
		clients, _, err := internetgateway1.NewWANIPConnection1Clients()

		if err != nil {
			log.Error("WAN Error discovering gateway, upnp likely not supported. %v", err)
		}
		if len(clients) == 0 {
			log.Error("***UPNP***WAN No gateway found, upnp likely not supported.")
		}
		for _, c := range clients {

			if host.Current.ListenPort != 0 && host.Current.Endpoint != "" {
				// get local ip address
				conn, err := net.Dial("udp", "8.8.8.8:53")
				if err != nil {
					log.Error("Impossible to get local ip address")
				} else {
					defer conn.Close()
					localAddr := conn.LocalAddr().(*net.UDPAddr)

					// get the external ip address
					externalIP, err := c.GetExternalIPAddress()
					if err != nil {
						log.Error("Error getting external ip address, %v", err)
					} else {
						log.Infof("***UPNP*** External IP address: %s", externalIP)
						// compare the externalIP to the endpoint
						parts := strings.Split(host.Current.Endpoint, ":")
						if parts[0] != externalIP && !isBogon(externalIP) {
							log.Error("External IP address does not match endpoint")
							// Update the host endpoint at nettica
							host.Current.Endpoint = externalIP + ":" + parts[1]
							UpdateHost(host)
						}
					}

					// delete any old port mappings
					err = c.DeletePortMapping("", uint16(host.Current.ListenPort), "UDP")
					if err != nil {
						log.Error("Error deleting port mapping, %v", err)
					}

					log.Infof("***UPNP*** AddPortMapping: %d %s %d %s %s", host.Current.ListenPort, "UDP", host.Current.ListenPort, localAddr.IP.String(), host.Name+"-"+host.NetName)
					// add port mapping
					err = c.AddPortMapping("", uint16(host.Current.ListenPort), "UDP", uint16(host.Current.ListenPort), localAddr.IP.String(), true, host.Name+"-"+host.NetName, 0)
					if err != nil {
						log.Error("Error adding port mapping, %v", err)
					}
				}
			}
		}

		ppp, _, err := internetgateway1.NewWANPPPConnection1Clients()
		if err != nil { // no ppp connection
			log.Error("Error discovering PPP gateway, likely doesn not exist. %v", err)
			return err
		}
		if len(ppp) == 0 {
			log.Error("No PPP gateway found, likely does not exist.")
			return err
		}

		log.Info("PPP gateway found, configuring external IP and port mapping.")

		for _, c := range ppp {

			if host.Current.ListenPort != 0 && host.Current.Endpoint != "" {
				// get local ip address
				conn, err := net.Dial("udp", "8.8.8.8:53")
				if err != nil {
					log.Error("Impossible to get local ip address")
				} else {
					defer conn.Close()
					localAddr := conn.LocalAddr().(*net.UDPAddr)

					// get the external ip address
					externalIP, err := c.GetExternalIPAddress()
					if err != nil {
						log.Error("PPP Error getting external ip address, %v", err)
					} else {
						log.Infof("***UPNP***PPP External IP address: %s", externalIP)
						// compare the externalIP to the endpoint
						parts := strings.Split(host.Current.Endpoint, ":")
						if parts[0] != externalIP && !isBogon(externalIP) {
							log.Error("PPP External IP address does not match endpoint")
							// Update the host endpoint at nettica
							host.Current.Endpoint = externalIP + ":" + parts[1]
							UpdateHost(host)
						}
					}

					// delete any old port mappings
					err = c.DeletePortMapping("", uint16(host.Current.ListenPort), "UDP")
					if err != nil {
						log.Error("PPP Error deleting port mapping, %v", err)
					}

					log.Infof("***UPNP***PPP AddPortMapping: %d %s %d %s %s", host.Current.ListenPort, "UDP", host.Current.ListenPort, localAddr.IP.String(), host.Name+"-"+host.NetName)
					// add port mapping
					err = c.AddPortMapping("", uint16(host.Current.ListenPort), "UDP", uint16(host.Current.ListenPort), localAddr.IP.String(), true, host.Name+"-"+host.NetName, 0)
					if err != nil {
						log.Error("PPP Error adding port mapping, %v", err)
					}
				}
			}
		}
	}

	return nil
}
