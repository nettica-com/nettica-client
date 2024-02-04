package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var netticaDeviceStatusAPIFmt = "%s/api/v1.0/device/%s/status"
var netticaDeviceAPIFmt = "%s/api/v1.0/device/%s"
var netticaVPNUpdateAPIFmt = "%s/api/v1.0/vpn/%s"

// Start the channel that iterates the nettica update function
func StartChannel(c chan []byte) {

	log.Infof("StartChannel Nettica Host %s", device.Server)
	etag := ""
	var err error

	for {
		content := <-c
		if content == nil {
			break
		}

		etag, err = GetNetticaVPN(etag)
		if err != nil {
			log.Errorf("Error getting nettica device: %v", err)
		}
	}
}

func DiscoverDevice(device *model.Device) {

	// don't do anything if the device is configured
	if device.Registered {
		return
	}

	found := false

	// AWS - check the metadata service
	// Get the instance ID from the metadata service
	// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html
	// GET http://169.254.169.254/latest/meta-data/instance-id

	req, err := http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
	if (err == nil) && (req != nil) {
		req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "300")
		rsp, err := http.DefaultClient.Do(req)

		if err == nil && rsp.StatusCode == 200 {
			body, err := io.ReadAll(rsp.Body)
			if err == nil {
				token := string(body)
				log.Infof("AWS Token: %s", token)
				rsp.Body.Close()

				req, err = http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/instance-id", nil)
				if (err == nil) && (req != nil) {
					req.Header.Set("X-aws-ec2-metadata-token", token)
					rsp, err = http.DefaultClient.Do(req)
					if err == nil && rsp.StatusCode == 200 {
						body, err := io.ReadAll(rsp.Body)
						if err == nil {
							device.InstanceID = string(body)
							log.Infof("AWS Instance ID: %s", device.InstanceID)
						}
						rsp.Body.Close()
						found = true
					}
				}
			} else {
				log.Infof("AWS Meta-data Error: %v", err)
				rsp.Body.Close()
			}
		} else {
			log.Infof("AWS Token Error: %v", err)
			rsp.Body.Close()
		}
	}

	// Azure - check the metadata service
	// GET http://169.254.169.254/metadata/instance/compute/vmId?api-version=2021-01-01&format=text

	if !found {
		req, err := http.NewRequest("GET", "http://169.254.169.254/metadata/instance/compute/vmId?api-version=2020-09-01&format=text", nil)
		if (err == nil) && (req != nil) {
			req.Header.Set("Metadata", "true")
			rsp, err := http.DefaultClient.Do(req)
			if err == nil && rsp.StatusCode == 200 {
				body, err := io.ReadAll(rsp.Body)
				if err == nil {
					device.InstanceID = string(body)
					log.Infof("Azure Instance ID: %s", device.InstanceID)
				}
				rsp.Body.Close()
				found = true
			}
		}
	}

	// Oracle - check the metadata service
	// GET curl -H "Authorization: Bearer Oracle" -L http://169.254.169.254/opc/v2/instance/id

	if !found {
		req, err := http.NewRequest("GET", "http://169.254.169.254/opc/v2/instance/id", nil)
		if (err == nil) && (req != nil) {
			req.Header.Set("Authorization", "Bearer Oracle")
			rsp, err := http.DefaultClient.Do(req)
			if err == nil && rsp.StatusCode == 200 {
				body, err := io.ReadAll(rsp.Body)
				if err == nil {
					device.InstanceID = string(body)
					log.Infof("Oracle Instance ID: %s", device.InstanceID)
				}
				rsp.Body.Close()
				found = true
			}
		}
	}

	if found {
		saveConfig()
	}

}

var client *http.Client

func CallNettica(etag *string) ([]byte, error) {

	server := device.Server

	if !device.Registered && device.InstanceID != "" {
		device.Id = "device-id-" + device.InstanceID
		if device.Server == "" {
			device.Server = "https://my.nettica.com"
		}
	} else if device.Server == "" || device.ApiKey == "" || device.Id == "" {
		// don't do anything if the device is not configured
		return nil, fmt.Errorf("no device configuration")
	}

	if client == nil {
		if strings.HasPrefix(server, "http:") {
			client = &http.Client{
				Timeout: time.Second * 10,
			}
		} else {
			// Create a transport like http.DefaultTransport, but with the configured LocalAddr
			transport := &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 60 * time.Second,
					LocalAddr: cfg.sourceAddr,
				}).Dial,
				TLSHandshakeTimeout: 10 * time.Second,
			}
			client = &http.Client{
				Transport: transport,
			}

		}
	}

	var reqURL string = fmt.Sprintf(netticaDeviceStatusAPIFmt, server, device.Id)
	if !device.Quiet {
		log.Infof("  GET %s", reqURL)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		client = nil
		return nil, err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-None-Match", *etag)
	}
	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode == 304 {
			buffer, err := os.ReadFile(GetDataPath() + "nettica.json")
			if err == nil {
				return buffer, nil
			}

		} else if resp.StatusCode == 401 {
			return nil, fmt.Errorf("Unauthorized")
		} else if resp.StatusCode == 404 {
			return nil, fmt.Errorf("not found")
		} else if resp.StatusCode != 200 {
			log.Errorf("Response Error Code: %v", resp.StatusCode)
			return nil, fmt.Errorf("response error code: %v", resp.StatusCode)
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Debugf("%s", string(body))

			etag2 := resp.Header.Get("ETag")

			if *etag != etag2 {
				log.Infof("etag = %s  etag2 = %s", *etag, etag2)
				*etag = etag2
			}
			resp.Body.Close()

			return body, nil
		}
	} else {
		log.Errorf("ERROR: %v, continuing", err)
		client = nil
	}

	return nil, err

}

func GetNetticaDevice() (*model.Device, error) {

	if !cfg.loaded {
		err := loadConfig()
		if err != nil {
			log.Errorf("Failed to load config.")
		}
	}

	server := device.Server
	var client *http.Client

	if strings.HasPrefix(server, "http:") {
		client = &http.Client{
			Timeout: time.Second * 10,
		}
	} else {
		// Create a transport like http.DefaultTransport, but with the configured LocalAddr
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
				LocalAddr: cfg.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaDeviceAPIFmt, server, device.Id)
	if !device.Quiet {
		log.Infof("  GET %s", reqURL)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode == 401 {
			return nil, fmt.Errorf("Unauthorized")
		} else if resp.StatusCode != 200 {
			log.Errorf("Response Error Code: %v", resp.StatusCode)
			return nil, fmt.Errorf("response error code: %v", resp.StatusCode)
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Debugf("%s", string(body))
			resp.Body.Close()

			// Marshall the JSON into a Device struct
			var d model.Device
			err = json.Unmarshal(body, &d)
			if err != nil {
				log.Errorf("error unmarshalling json %v", err)
			}

			return &d, nil
		}
	} else {
		log.Errorf("ERROR: %v, continuing", err)
	}

	return nil, err
}

func DeleteVPN(id string) error {

	var client *http.Client

	if strings.HasPrefix(device.Server, "http:") {
		client = &http.Client{
			Timeout: time.Second * 10,
		}
	} else {
		// Create a transport like http.DefaultTransport, but with the configured LocalAddr
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
				LocalAddr: cfg.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaVPNUpdateAPIFmt, device.Server, id)
	if !device.Quiet {
		log.Infof("  DELETE %s", reqURL)
	}

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode == 401 {
			return fmt.Errorf("Unauthorized")
		} else if resp.StatusCode != 200 {
			log.Errorf("Response Error Code: %v", resp.StatusCode)
			return fmt.Errorf("response error code: %v", resp.StatusCode)
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Debugf("%s", string(body))
			resp.Body.Close()

			return nil
		}
	} else {
		log.Errorf("ERROR: %v, continuing", err)
	}

	return err

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

	if d1.Name != d2.Name {
		return false
	}

	if d1.ApiKey != d2.ApiKey {
		return false
	}

	if d1.Server != d2.Server {
		return false
	}

	if d1.Quiet != d2.Quiet {
		return false
	}

	if d1.Debug != d2.Debug {
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

	if d1.ApiID != d2.ApiID {
		return false
	}

	if d1.ClientID != d2.ClientID {
		return false
	}

	if d1.AppData != d2.AppData {
		return false
	}

	if d1.AuthDomain != d2.AuthDomain {
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

	return true
}

// function merges two devices, d1 is the source, d2 is the destination
func MergeDevices(d1 *model.Device, d2 *model.Device) {

	if (d1 == nil) || (d2 == nil) {
		return
	}

	// Some properties, like Quiet and Debug, cannot be controlled by the server
	// InstanceID is not managed by the server

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

	if d1.Version != "" {
		d2.Version = d1.Version
	}

	if d1.SourceAddress != "" {
		d2.SourceAddress = d1.SourceAddress
	}

	d2.Updated = d1.Updated
	d2.Created = d1.Created

	if d1.ApiID != "" {
		d2.ApiID = d1.ApiID
	}

	if d1.ClientID != "" {
		d2.ClientID = d1.ClientID
	}

	if d1.AppData != "" {
		d2.AppData = d1.AppData
	}

	if d1.AuthDomain != "" {
		d2.AuthDomain = d1.AuthDomain
	}

	if d1.AccountID != "" {
		d2.AccountID = d1.AccountID
	}

	if d1.ServiceGroup != "" {
		d2.ServiceGroup = d1.ServiceGroup
	}

	if d1.ServiceApiKey != "" {
		d2.ServiceApiKey = d1.ServiceApiKey
	}

}

func UpdateNetticaDevice(d model.Device) error {

	log.Infof("UPDATING DEVICE: %v", d)

	server := device.Server
	var client *http.Client

	if strings.HasPrefix(server, "http:") {
		client = &http.Client{
			Timeout: time.Second * 10,
		}
	} else {
		// Create a transport like http.DefaultTransport, but with the configured LocalAddr
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
				LocalAddr: cfg.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaDeviceAPIFmt, server, d.Id)
	log.Infof("  PATCH %s", reqURL)
	content, err := json.Marshal(d)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", reqURL, bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode != 200 {
			log.Errorf("PATCH Error: Response %v", resp.StatusCode)
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Infof("%s", string(body))
		}
	}

	if resp != nil {
		resp.Body.Close()
	}
	if req != nil {
		req.Body.Close()
	}

	return nil
}

func GetNetticaVPN(etag string) (string, error) {

	if !cfg.loaded {
		err := loadConfig()
		if err != nil {
			log.Errorf("Failed to load config.")
		}
	}

	body, err := CallNettica(&etag)
	if err != nil {
		client = nil
		if err.Error() == "Unauthorized" {
			log.Errorf("Unauthorized - reload config")
			// Read the config and find another API key
			// pick up any changes from the agent or manually editing the config file.
			reloadConfig()
		} else if err.Error() == "not found" {
			log.Errorf("Device not found")
		} else {
			log.Error(err)
		}
	} else {
		UpdateNetticaConfig(body)
		return etag, nil
	}

	return "", err
}

func UpdateVPN(vpn *model.VPN) error {

	log.Infof("UPDATING VPN: %v", vpn)
	server := device.Server
	var client *http.Client

	if strings.HasPrefix(server, "http:") {
		client = &http.Client{
			Timeout: time.Second * 10,
		}
	} else {
		// Create a transport like http.DefaultTransport, but with the configured LocalAddr
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
				LocalAddr: cfg.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaVPNUpdateAPIFmt, server, vpn.Id)
	log.Infof("  PATCH %s", reqURL)
	content, err := json.Marshal(vpn)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", reqURL, bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode != 200 {
			log.Errorf("PATCH Error: Response %v", resp.StatusCode)
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Infof("%s", string(body))
		}
	}

	if resp != nil {
		resp.Body.Close()
	}
	if req != nil {
		req.Body.Close()
	}

	return nil
}

// UpdateNetticaConfig updates the config from the server
func UpdateNetticaConfig(body []byte) {

	// If the file doesn't exist create it for the first time
	if _, err := os.Stat(GetDataPath() + "nettica.json"); os.IsNotExist(err) {
		file, err := os.OpenFile(GetDataPath()+"nettica.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err == nil {
			file.Close()
		}
	}

	file, err := os.Open(GetDataPath() + "nettica.json")

	if err != nil {
		log.Errorf("Error opening nettica.json file %v", err)
		return
	}
	conf, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading nettica config file: %v", err)
		return
	}

	// compare the body to the current config and make no changes if they are the same
	if bytes.Equal(conf, body) {
		return
	} else {
		log.Info("Config has changed, updating nettica.json")

		// if we can't read the message, immediately return
		var msg model.Message
		err = json.Unmarshal(body, &msg)
		if err != nil {
			log.Errorf("Error reading message from server")
			return
		}

		file, err := os.OpenFile(GetDataPath()+"nettica.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			log.Errorf("Error opening nettica.json for write: %v", err)
			return
		}
		_, err = file.Write(body)
		file.Close()
		if err != nil {
			log.Infof("Error writing nettica.json file: %v", err)
			return
		}

		var oldconf model.Message
		err = json.Unmarshal(conf, &oldconf)
		if err != nil {
			log.Errorf("Error reading old config: %v", err)
			log.Infof("Old config: %v", string(conf))
		}

		log.Debugf("%v", msg)

		// See if the device is enabled.  If its not, stop all networks and return
		if (msg.Device != nil) && !msg.Device.Enable {
			log.Infof("Device is disabled, stopping all networks")
			for i := 0; i < len(msg.Config); i++ {
				StopWireguard(msg.Config[i].NetName)
			}
			MergeDevices(msg.Device, &device)
			saveConfig()
			return
		}

		if (msg.Device.CheckInterval != 0) && (msg.Device.CheckInterval != device.CheckInterval) {
			log.Infof("CheckInterval has changed, new interval is %d", msg.Device.CheckInterval)
			device.CheckInterval = msg.Device.CheckInterval
			saveConfig()
		}

		if msg.Device.Quiet != device.Quiet {
			log.Infof("Quiet has changed, new quiet is %t", msg.Device.Quiet)
			device.Quiet = msg.Device.Quiet
			saveConfig()
		}

		if msg.Device.Description != device.Description {
			log.Infof("Description has changed, new description is %s", msg.Device.Description)
			device.Description = msg.Device.Description
			saveConfig()
		}

		if !slices.Equal(msg.Device.Tags, device.Tags) {
			log.Infof("Tags have changed, new tags are %v", msg.Device.Tags)
			device.Tags = msg.Device.Tags
			saveConfig()
		}

		if msg.Device.Server != device.Server {
			log.Infof("Server has changed, new server is %s", msg.Device.Server)
			device.Server = msg.Device.Server
			saveConfig()
		}

		// One-Time discovery of the device
		if device.ApiKey == "" && msg.Device.ApiKey != "" {
			log.Infof("Device is not registered, registering")
			device.ApiKey = msg.Device.ApiKey
			device.Id = msg.Device.Id
			device.Registered = true
			saveConfig()
		}

		// make a copy of the message since UpdateDNS will alter it.
		var msg2 model.Message
		json.Unmarshal(body, &msg2)
		err = UpdateDNS(msg2)
		if err != nil {
			log.Errorf("Error updating DNS configuration: %v", err)
		}

		// Get our local subnets, called here to avoid duplication
		subnets, err := GetLocalSubnets()
		if err != nil {
			log.Errorf("GetLocalSubnets, err = %v", err)
		}

		// first, delete any nets that are no longer in the conf
		for i := 0; i < len(oldconf.Config); i++ {
			found := false
			for j := 0; j < len(msg.Config); j++ {
				if oldconf.Config[i].NetName == msg.Config[j].NetName {
					found = true
					break
				}
			}
			if !found {
				log.Infof("Deleting net %v", oldconf.Config[i].NetName)
				RemoveWireguard(oldconf.Config[i].NetName)
				os.Remove(GetDataPath() + oldconf.Config[i].NetName + ".conf")

				for _, vpn := range oldconf.Config[i].VPNs {
					if vpn.DeviceID == device.Id {
						KeyDelete(vpn.Current.PublicKey)
						KeySave()
					}
				}
				msg := fmt.Sprintf("Network %s has been removed", oldconf.Config[i].NetName)
				NotifyInfo(msg)
			}
		}

		// handle any other changes
		for i := 0; i < len(msg.Config); i++ {
			index := -1
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == device.Id {
					index = j
					break
				}
			}
			if index == -1 {
				log.Errorf("Error reading message %v", msg)
			} else {
				vpn := msg.Config[i].VPNs[index]
				msg.Config[i].VPNs = append(msg.Config[i].VPNs[:index], msg.Config[i].VPNs[index+1:]...)

				// Configure UPnP as needed
				go ConfigureUPnP(vpn)

				// If any of the AllowedIPs contain our subnet, remove that entry
				for k := 0; k < len(msg.Config[i].VPNs); k++ {
					allowed := msg.Config[i].VPNs[k].Current.AllowedIPs
					for l := 0; l < len(allowed); l++ {
						inSubnet := false
						_, s, _ := net.ParseCIDR(allowed[l])
						for _, subnet := range subnets {
							if subnet.Contains(s.IP) {
								inSubnet = true
							}
						}
						if inSubnet {
							msg.Config[i].VPNs[k].Current.AllowedIPs = append(allowed[:l], allowed[l+1:]...)
						}
					}
				}

				// Check to see if we have the private key

				key, found := KeyLookup(vpn.Current.PublicKey)
				if !found {
					KeyAdd(vpn.Current.PublicKey, vpn.Current.PrivateKey)
					err = KeySave()
					if err != nil {
						log.Errorf("Error saving key: %s %s", vpn.Current.PublicKey, vpn.Current.PrivateKey)
					}

					if device.UpdateKeys {
						// clear out the private key and update the server
						vpn2 := vpn
						vpn2.Current.PrivateKey = ""
						UpdateVPN(&vpn2)
					}

					key, _ = KeyLookup(vpn.Current.PublicKey)
				}

				// If the private key is blank create a new one and update the server
				if key == "" {
					// delete the old public key
					KeyDelete(vpn.Current.PublicKey)
					wg, _ := wgtypes.GeneratePrivateKey()
					vpn.Current.PrivateKey = wg.String()
					vpn.Current.PublicKey = wg.PublicKey().String()
					KeyAdd(vpn.Current.PublicKey, vpn.Current.PrivateKey)
					KeySave()

					vpn2 := vpn
					vpn2.Current.PrivateKey = ""

					// Update nettica with the new public key
					UpdateVPN(&vpn2)

				} else {
					vpn.Current.PrivateKey = key
				}

				text, err := DumpWireguardConfig(&vpn, &(msg.Config[i].VPNs))
				if err != nil {
					log.Errorf("error on template: %s", err)
				}

				// Check the current file and if it's an exact match, do not bounce the service
				path := GetWireguardPath()
				name := msg.Config[i].NetName

				force := false
				var bits []byte

				file, err := os.Open(path + name + ".conf")
				if err != nil {
					log.Errorf("Error opening %s for read: %v", name, err)
					force = true
				} else {
					bits, err = io.ReadAll(file)
					file.Close()
					if err != nil {
						log.Errorf("Error reading nettica config file: %v", err)
						force = true
					}
				}

				if !force && bytes.Equal(bits, text) {
					log.Infof("*** SKIPPING %s *** No changes!", name)
				} else {
					err = StopWireguard(name)
					if err != nil {
						log.Errorf("Error stopping wireguard: %v", err)
					}

					// Stop the DNS if is running
					if vpn.Current.EnableDns {
						address := vpn.Current.Address[0]
						address = address[0:strings.Index(address, "/")]
						StopDNS(address)
					}

					err = os.MkdirAll(path, 0600)
					if err != nil {
						log.Errorf("Error creating directory %s : %s", path, err)
					}

					err = os.WriteFile(path+name+".conf", text, 0600)
					if err != nil {
						log.Errorf("Error writing file %s : %s", path+name+".conf", err)
					}

					if !vpn.Enable {
						// Host was disabled when we stopped wireguard above
						log.Infof("Net %s is disabled.  Stopped service if running.", name)
						// Stopping the service doesn't seem very reliable, stop it again
						if err = StopWireguard(name); err != nil {
							log.Errorf("Error stopping wireguard: %v", err)
						}

						msg := fmt.Sprintf("Network %s has been stopped", name)
						NotifyInfo(msg)

					} else {
						// Start the existing service
						err = StartWireguard(name)
						if err == nil {
							log.Infof("Started %s", name)
							log.Infof("%s Config: %v", name, msg.Config[i])
							msg := fmt.Sprintf("Network %s has been updated", name)
							NotifyInfo(msg)
						} else {
							// try to install the service (on linux, just tries to start the service again)
							err = InstallWireguard(name)
							if err != nil {
								log.Errorf("Error installing wireguard: %v", err)
							} else {
								log.Infof("Installed %s Config: %v", name, msg.Config[i])
								msg := fmt.Sprintf("Network %s has been installed", name)
								NotifyInfo(msg)
							}
						}
					}
				}

			}
		}
	}

}

func FindVPN(net string) (*model.VPN, error) {

	file, err := os.Open(GetDataPath() + "nettica.json")

	if err != nil {
		log.Errorf("Error opening nettica.json file %v", err)
		return nil, err
	}
	conf, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading nettica config file: %v", err)
		return nil, err
	}

	var msg model.Message
	err = json.Unmarshal(conf, &msg)
	if err != nil {
		log.Errorf("Error reading message from config file")
		return nil, err
	}

	for i := 0; i < len(msg.Config); i++ {
		if msg.Config[i].NetName == net {
			log.Infof("Found net %s", net)
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == device.Id {
					log.Infof("Found VPN %v", msg.Config[i].VPNs[j])
					return &msg.Config[i].VPNs[j], nil
				}
			}
		}
	}

	return nil, fmt.Errorf("VPN not found")

}

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

// This needs to be refactored with the main logic above
func StartBackgroundRefreshService() {

	// Wait for the main thread to contact Nettica at least once before starting up the VPNs
	// on a reboot

	time.Sleep(15 * time.Second)

	for {

		file, err := os.Open(GetDataPath() + "nettica.json")
		if err != nil {
			log.Errorf("Error opening nettica.json for read: %v", err)
			return
		}
		bytes, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			log.Errorf("Error reading nettica config file: %v", err)
			return
		}
		var msg model.Message
		err = json.Unmarshal(bytes, &msg)
		if err != nil {
			log.Errorf("Error reading message from server")
		}

		log.Debugf("%v", msg)

		// See if the device is enabled.  If its not, stop all networks and return
		if (msg.Device != nil) && !msg.Device.Enable {
			log.Infof("Device is disabled, stopping all networks")
			for i := 0; i < len(msg.Config); i++ {
				StopWireguard(msg.Config[i].NetName)
			}
			MergeDevices(msg.Device, &device)
			saveConfig()
			return
		}

		// Get our local subnets, called here to avoid duplication
		subnets, err := GetLocalSubnets()
		if err != nil {
			log.Errorf("GetLocalSubnets, err = %v", err)
		}

		for i := 0; i < len(msg.Config); i++ {
			index := -1
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == device.Id {
					index = j
					break
				}
			}
			if index == -1 {
				log.Errorf("Error reading message %v", msg)
			} else {
				vpn := msg.Config[i].VPNs[index]
				msg.Config[i].VPNs = append(msg.Config[i].VPNs[:index], msg.Config[i].VPNs[index+1:]...)

				// Configure UPnP as needed
				go ConfigureUPnP(vpn)

				// If any of the AllowedIPs contain our subnet, remove that entry
				for k := 0; k < len(msg.Config[i].VPNs); k++ {
					allowed := msg.Config[i].VPNs[k].Current.AllowedIPs
					for l := 0; l < len(allowed); l++ {
						inSubnet := false
						_, s, _ := net.ParseCIDR(allowed[l])
						for _, subnet := range subnets {
							if subnet.Contains(s.IP) {
								inSubnet = true
							}
						}
						if inSubnet {
							msg.Config[i].VPNs[k].Current.AllowedIPs = append(allowed[:l], allowed[l+1:]...)
						}
					}
				}
				// Check to see if we have the private key

				key, found := KeyLookup(vpn.Current.PublicKey)
				if !found {
					KeyAdd(vpn.Current.PublicKey, vpn.Current.PrivateKey)
					err = KeySave()
					if err != nil {
						log.Errorf("Error saving key: %s %s", vpn.Current.PublicKey, vpn.Current.PrivateKey)
					}
					key, _ = KeyLookup(vpn.Current.PublicKey)
				}

				// If the private key is blank create a new one and update the server
				if key == "" {
					// delete the old public key
					KeyDelete(vpn.Current.PublicKey)
					wg, _ := wgtypes.GeneratePrivateKey()
					vpn.Current.PrivateKey = wg.String()
					vpn.Current.PublicKey = wg.PublicKey().String()
					KeyAdd(vpn.Current.PublicKey, vpn.Current.PrivateKey)
					KeySave()

					vpn2 := vpn
					vpn2.Current.PrivateKey = ""

					// Update nettica with the new public key
					UpdateVPN(&vpn2)

				} else {
					vpn.Current.PrivateKey = key
				}

				text, err := DumpWireguardConfig(&vpn, &(msg.Config[i].VPNs))
				if err != nil {
					log.Errorf("error on template: %s", err)
				}
				path := GetWireguardPath()
				err = os.WriteFile(path+msg.Config[i].NetName+".conf", text, 0600)
				if err != nil {
					log.Errorf("Error writing file %s : %s", path+msg.Config[i].NetName+".conf", err)
				}

				if !vpn.Enable {
					StopWireguard(msg.Config[i].NetName)
					log.Infof("Net %s is disabled.  Stopped service if running.", msg.Config[i].NetName)
				} else {
					err = StartWireguard(msg.Config[i].NetName)
					if err == nil {
						log.Infof("Started %s", msg.Config[i].NetName)
						log.Infof("%s Config: %v", msg.Config[i].NetName, msg.Config[i])
					} else {
						err = InstallWireguard(msg.Config[i].NetName)
						if err != nil {
							log.Errorf("Error installing wireguard: %v", err)
						}
					}

				}

			}
			StartDNS()
		}
		// Do this startup process every hour.  Keeps UPnP ports active, handles laptop sleeps, etc.
		time.Sleep(60 * time.Minute)
	}
}

// DoWork error handler
func DoWork() {
	var curTs int64

	// recover from any panics coming from below
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok := r.(error)
			if !ok {
				log.Fatalf("Fatal Error: %v", err)
			}
		}
	}()

	go func() {

		// Determine current timestamp (the wallclock time we'll retrieve files using)

		c := make(chan []byte)
		go startHTTPd()
		go StartChannel(c)
		go StartDNS()
		go StartBackgroundRefreshService()

		curTs = calculateCurrentTimestamp()

		t := time.Unix(curTs, 0)
		log.Infof("current timestamp = %v (%s)", curTs, t.UTC())

		for {
			time.Sleep(1000 * time.Millisecond)
			ts := time.Now()

			if ts.Unix() >= curTs {

				b := []byte("")

				c <- b

				curTs = calculateCurrentTimestamp()
				curTs += device.CheckInterval
			}

		}
	}()
}

func calculateCurrentTimestamp() int64 {

	return time.Now().Unix()

}
