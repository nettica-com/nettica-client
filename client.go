package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// Global variable for status changes
var (
	Bounce          = false
	FailSafe        = false
	FailSafeActed   = false
	FailSafeMsgSent = false
	Count           = 0
)

const (
	NOFAILOVER = 0
	FAILOVER   = 1
)

// Nettica API URLs
var (
	netticaDeviceStatusAPIFmt = "%s/api/v1.0/device/%s/status"
	netticaDeviceAPIFmt       = "%s/api/v1.0/device/%s"
	netticaVPNUpdateAPIFmt    = "%s/api/v1.0/vpn/%s"
)

// Start the channel that iterates the nettica update function
func StartChannel(c chan []byte) {

	log.Infof("StartChannel Nettica Host %s", device.Server)
	etag := ""
	success := true
	var err error
	localIP, err := GetLocalIP()
	if err != nil {
		log.Errorf("Error getting local ip address: %v", err)
	}

	for {
		content := <-c
		if content == nil {
			break
		}

		ip, err := GetLocalIP()
		if err != nil {
			log.Errorf("Error getting local ip address: %v", err)
		} else if ip != localIP {
			msg := fmt.Sprintf("Local IP address has changed from %s to %s.  Checking for updates...", localIP, ip)
			NotifyInfo(msg)
			Bounce = true
			localIP = ip
		}

		etag, err = GetNetticaVPN(etag)
		if err != nil {
			log.Errorf("Error getting nettica device: %v", err)
			success = false
			Count++
			if Count > 3 {
				FailSafe = true
				log.Info("FailSafe mode enabled.")
				Failsafe()
			}
		} else {
			if !success && !Bounce {
				success = true
				Bounce = true
				NotifyInfo("Network change detected.  Checking for updates...")
			} else {
				if FailSafe {
					if FailSafeActed {
						NotifyInfo("FailSafe has recovered connectivity")
					}
				}
			}
			FailSafe = false
			FailSafeActed = false
			FailSafeMsgSent = false
			Count = 0
		}
	}
}

func Failsafe() error {

	file, err := os.Open(GetDataPath() + "nettica.json")

	if err != nil {
		log.Errorf("Error opening nettica.json file %v", err)
		return err
	}
	conf, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading nettica config file: %v", err)
		return err
	}

	UpdateNetticaConfig(conf)

	return err
}

// Get the current Source Address
func GetLocalIP() (string, error) {

	ip := ""

	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		log.Error("Impossible to get local ip address")
	} else {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		ip = localAddr.IP.String()
	}

	return ip, err
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

	if !device.Registered && (device.InstanceID != "" || device.EZCode != "") {
		if strings.HasPrefix(device.EZCode, "ez-") {
			device.Id = device.EZCode
		} else {
			device.Id = "device-id-" + device.InstanceID
		}
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

	answer, err := net.LookupIP("my.nettica.com")
	if err != nil {
		log.Errorf("DNS lookup for my.nettica.com failed: %v", err)
	}

	var reqURL string = fmt.Sprintf(netticaDeviceStatusAPIFmt, server, device.Id)
	if !device.Quiet {
		s := "NXDOMAIN"
		if len(answer) > 0 {
			s = answer[0].String()
		}
		log.Infof("  GET %s (%s)", reqURL, s)
	}

	// Create a context with a 15 second timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(15*time.Second))
	defer cancel()

	// make a DNS lookup for my.nettica.com

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		client = nil
		return nil, err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/"+Version)
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

	// Create a context with a 15 second timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(15*time.Second))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/"+Version)
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

	// Create a context with a 15 second timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(15*time.Second))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/"+Version)
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

func UpdateNetticaDevice(d model.Device) error {

	log.Infof("UPDATING DEVICE: %v", d)

	if device.Server == "" || device.AccountID == "" || device.ApiKey == "" || device.Id == "" {
		return errors.New("skipping update, not enough information.  waiting for server to update us")
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

	var reqURL string = fmt.Sprintf(netticaDeviceAPIFmt, server, d.Id)
	log.Infof("  PATCH %s", reqURL)
	content, err := json.Marshal(d)
	if err != nil {
		return err
	}

	// Create a context with a 15 second timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(15*time.Second))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "PATCH", reqURL, bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/"+Version)
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
		if FailSafe {
			NotifyInfo("FailSafe: connectivity restored")
			FailSafe = false
			FailSafeActed = false
			FailSafeMsgSent = false
			Count = 0
		}
		UpdateNetticaConfig(body)
		return etag, nil
	}

	return "", err
}

func UpdateVPN(vpn *model.VPN) error {

	log.Infof(" ******************** UPDATING VPN: %s ********************", vpn.Name)
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

	// Create a context with a 15 second timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(15*time.Second))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "PATCH", reqURL, bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", device.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/"+Version)
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

	defer func() {
		Bounce = false
	}()

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
	if bytes.Equal(conf, body) && !Bounce && !FailSafe {
		return
	} else {
		if Bounce {
			log.Info("BOUNCE!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		}

		if FailSafe {
			log.Info("FailSafe!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		}

		log.Info("Config has changed, updating nettica.json")

		// if we can't read the message, immediately return
		var msg model.Message
		err = json.Unmarshal(body, &msg)
		if err != nil {
			log.Errorf("Error reading message from server")
			return
		}

		// If this is a ServiceHost, validate the message before proceeding
		if ServiceHost {
			err := ValidateMessage(&msg)
			if err != nil {
				log.Errorf("Error validating message: %v", err)
				return
			}
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

		log.Debugf("Server Msg: %v", msg)

		// See if the device is enabled.  If its not, stop all networks and return
		if (msg.Device != nil) && !msg.Device.Enable {
			log.Error("Device is disabled, stopping all networks")
			for i := 0; i < len(msg.Config); i++ {
				log.Errorf("Stopping %s", msg.Config[i].NetName)
				StopWireguard(msg.Config[i].NetName)
			}
			MergeDevices(msg.Device, &device)
			saveConfig()
			return
		}

		if msg.Device.Quiet != device.Quiet {
			log.Errorf("Quiet has changed, new quiet is %t", msg.Device.Quiet)
			device.Quiet = msg.Device.Quiet
			saveConfig()
		}

		if (msg.Device.CheckInterval != 0) && (msg.Device.CheckInterval != device.CheckInterval) {
			log.Infof("CheckInterval has changed, new interval is %d", msg.Device.CheckInterval)
			device.CheckInterval = msg.Device.CheckInterval
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

		if (msg.Device.Name != "") && (msg.Device.Name != device.Name) &&
			msg.Device.Updated.After(device.Updated) {
			log.Infof("Name has changed, new name is %s", msg.Device.Name)
			device.Name = msg.Device.Name
			saveConfig()
		}

		if msg.Device.AccountID != "" && msg.Device.AccountID != device.AccountID {
			device.AccountID = msg.Device.AccountID
			saveConfig()
		}

		// make a copy of the message since UpdateDNS will alter it.
		var msg2 model.Message
		json.Unmarshal(body, &msg2)

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

		// force a network bounce under certain conditions
		// moved outside loop for reprocessing a network that
		// might otherwise be missed
		force := false

		//
		// handle any other changes
		//

		// msg schema is:
		// type Message struct {
		// 	Device *Device `json:"device"`
		// 	Config []Net   `json:"config"`
		// }
		// type Net struct {
		// 	NetName string `json:"netname"`
		// 	VPNs    []VPN  `json:"vpns"`
		// }
		// type VPN struct {
		// 	DeviceID string   `json:"deviceid"`
		// 	Name     string   `json:"name"`
		// 	Current  Settings `json:"current"`
		// 	Enable   bool     `json:"enable"`
		// 	FailSafe bool     `json:"failsafe"`
		// 	Failover int      `json:"failover"`
		// }

		// loop through the networks
		for i := 0; i < len(msg.Config); i++ {
			index := -1
			// find our VPN in the configuration
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == device.Id {
					index = j
					break
				}
			}
			if index == -1 {
				log.Errorf("Error reading message %v", msg)
			} else {
				// physically pull out our VPN from the other configurations
				vpn := msg.Config[i].VPNs[index]
				msg.Config[i].VPNs = append(msg.Config[i].VPNs[:index], msg.Config[i].VPNs[index+1:]...)

				// Configure UPnP as needed
				go ConfigureUPnP(vpn)

				// Get our local subnets
				subnets, err := GetLocalSubnets()
				if err != nil {
					log.Errorf("GetLocalSubnets, err = %v", err)
				}
				log.Errorf("Subnets: %v", subnets)

				// Iterate through this VPN's addresses and remove any subnets that match
				// What should be left is the subnets that are local to this device
				for k := 0; k < len(vpn.Current.Address); k++ {
					network, err := GetNetworkAddress(vpn.Current.Address[k])
					if err != nil {
						log.Errorf("GetNetworkAddress, err = %v", err)
						continue
					}
					for l := 0; l < len(subnets); l++ {
						if subnets[l].String() == network {
							log.Errorf("From Local: Removing subnet %s from %s", subnets[l].String(), vpn.Name)
							subnets = append(subnets[:l], subnets[l+1:]...)
						}
					}
				}

				// If any of the AllowedIPs contain a local subnet, remove that entry
				// This is to prevent routing loops and is very important
				for k := 0; k < len(msg.Config[i].VPNs); k++ {
					allowed := msg.Config[i].VPNs[k].Current.AllowedIPs
					for l := 0; l < len(allowed); l++ {
						inSubnet := false
						if !strings.Contains(allowed[l], "/") {
							continue
						}
						_, s, err := net.ParseCIDR(allowed[l])
						if err != nil {
							log.Errorf("net.ParseCIDR err = %v", err)
							continue
						}
						for _, subnet := range subnets {
							if subnet.Contains(s.IP) {
								log.Errorf("From Foreign: Removing subnet %s from %s", allowed[l], vpn.Name)
								inSubnet = true
							}
						}
						if inSubnet {
							msg.Config[i].VPNs[k].Current.AllowedIPs = append(allowed[:l], allowed[l+1:]...)
						}
					}
				}

				// Check to see if we have a private key for this public key
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

				// Create a new NetName.conf configuration file
				text, err := DumpWireguardConfig(&vpn, &(msg.Config[i].VPNs))
				if err != nil {
					log.Errorf("error on template: %s", err)
				}

				// Check the current file and if it's an exact match, do not bounce the service
				path := GetWireguardPath()
				name := msg.Config[i].NetName

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

				// FailSafe processing
				if FailSafe {
					log.Infof("FailSafe: %v vpn.FailSafe %v Enable %v Failover %v", FailSafe, vpn.Current.FailSafe, vpn.Enable, vpn.Failover)
				}

				// If we're in FailSafe, and the VPN is configured for it, and the VPN is enabled...
				if FailSafe && vpn.Current.FailSafe && vpn.Enable {
					// Check to see if the VPN is running
					running, err := IsWireguardRunning(name)
					if err != nil {
						log.Errorf("Error checking wireguard: %v", err)
					}

					// If the VPN is running, stop it, and the DNS if it's enabled
					if running {

						log.Infof("FailSafe: %s failed.  Stopping service", name)
						if !FailSafeMsgSent {
							msg := fmt.Sprintf("FailSafe: Network %s failed. Stopping service", name)
							NotifyInfo(msg)
						}

						// Stop the DNS if is running
						if vpn.Current.EnableDns {
							address := vpn.Current.Address[0]
							if strings.Contains(address, "/") {
								address = address[0:strings.Index(address, "/")]
							}
							StopDNS(address)
							// Since the DNS has a shared cache, we need to dump the whole thing.
							// On recovery we'll rebuild it.
							DropCache()
						}

						// Stop WireGuard
						err = StopWireguard(name)
						if err != nil {
							log.Errorf("Error stopping wireguard: %v", err)
						}

						FailSafeActed = true
						vpn.Failover = FAILOVER

						// Update the server with any luck
						if err = UpdateVPN(&vpn); err != nil {
							log.Errorf("Error updating VPN: %v", err)
						}
					} else {

						// If the VPN is not running, maybe it should be.  Start it.
						log.Infof("FailSafe: Starting network %s", name)
						if !FailSafeMsgSent {
							msg := fmt.Sprintf("FailSafe: Starting network %s", name)
							NotifyInfo(msg)
						}

						err = StartWireguard(name)
						if err != nil {
							log.Errorf("Error starting wireguard: %v", err)
						}

						FailSafeActed = true

					}
					log.Infof(" >>>>>>>>>> Failover processing for %s <<<<<<<<<<", name)

					// Skip the rest of this processing because it's for normal operations
					continue
				}

				if !force && bytes.Equal(bits, text) {
					log.Infof("*** SKIPPING %s *** No changes!", name)
				} else {
					// reinitialize force to false for future iterations
					force = false

					// The configuration has changed, so stop the service to perform the update

					// Stop the DNS if is running
					if vpn.Current.EnableDns {
						address := vpn.Current.Address[0]
						if strings.Contains(address, "/") {
							address = address[0:strings.Index(address, "/")]
						}
						StopDNS(address)
					}

					err = StopWireguard(name)
					if err != nil {
						log.Errorf("Error stopping wireguard: %v", err)
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
						} else {
							log.Infof("Stopped %s", name)
							msg := fmt.Sprintf("Network %s has been stopped", name)
							NotifyInfo(msg)
						}
					} else {
						// Start the existing service
						err = StartWireguard(name)
						if err == nil {
							log.Infof("Started %s", name)
							// log.Infof("%s Config: %v", name, msg.Config[i])
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
		// After everything has been processed, update the DNS
		// We do it here to avoid multiple updates
		err = UpdateDNS(msg2)
		if err != nil {
			log.Errorf("Error updating DNS configuration: %v", err)
		}
	}

	if FailSafe {
		FailSafeMsgSent = true
	}

}

// ServiceHost containers must be hardened to prevent unauthorized access to the host
// Check the following:
// 1. There is only one VPN for this device
// 2. The PreUp and PostDown scripts are not set
// 3. The PostUp and PostDown don't have specific commands
// 4. The Type of VPN is set to Service
// 5. The overall Device and VPN conform to their models
// This will allow the host to be used in the wild without fear of exploitation
func ValidateMessage(msg *model.Message) error {

	if !ServiceHost {
		return nil
	}

	if len(msg.Config) != 1 {
		return fmt.Errorf("only one network is allowed for this device")
	}

	errs := msg.Device.IsValid()
	if len(errs) > 0 {
		return fmt.Errorf("device is not valid: %v", errs)
	}

	// Check the VPN
	index := -1
	deviceid := msg.Device.Id
	for i := 0; i < len(msg.Config); i++ {
		for j := 0; j < len(msg.Config[i].VPNs); j++ {
			if msg.Config[i].VPNs[j].DeviceID == deviceid {
				index = j
				break
			}
		}

		if index == -1 {
			return fmt.Errorf("vpn not found")
		}

		vpn := &msg.Config[i].VPNs[index]
		errs = vpn.IsValid()
		if len(errs) > 0 {
			return fmt.Errorf("vpn is not valid: %v", errs)
		}

		if vpn.Type != "Service" {
			return fmt.Errorf("invalid VPN type")
		}

		// Check PreUp and PreDown
		if vpn.Current.PreUp != "" || vpn.Current.PreDown != "" {
			return fmt.Errorf("invalid PreUp and PreDown")
		}

		// Check PostUp and PostDown
		postUp := Sanitize(vpn.Current.PostUp)
		postDown := Sanitize(vpn.Current.PostDown)
		if postUp != vpn.Current.PostUp || postDown != vpn.Current.PostDown {
			return fmt.Errorf("invalid PostUp and PostDown")
		}

		// Check for reserved words
		// Note: The docker container doesn't have much installed to exploit
		// and checking for apk ensures it stays that way
		ReservedWords := []string{"apk", "ssh", "wget", "curl"}
		for _, word := range ReservedWords {
			if strings.Contains(postUp, word) || strings.Contains(postDown, word) {
				return fmt.Errorf("invalid PostUp and PostDown")
			}
		}

	}

	return nil
}

func FindVPN(net string) (*model.VPN, *[]model.VPN, error) {

	file, err := os.Open(GetDataPath() + "nettica.json")

	if err != nil {
		log.Errorf("Error opening nettica.json file %v", err)
		return nil, nil, err
	}
	conf, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading nettica config file: %v", err)
		return nil, nil, err
	}

	var msg model.Message
	err = json.Unmarshal(conf, &msg)
	if err != nil {
		log.Errorf("Error reading message from config file")
		return nil, nil, err
	}

	for i := 0; i < len(msg.Config); i++ {
		if msg.Config[i].NetName == net {
			log.Infof("Found net %s", net)
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == device.Id {
					log.Infof("Found VPN %v", msg.Config[i].VPNs[j])
					return &msg.Config[i].VPNs[j], &msg.Config[i].VPNs, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("VPN not found")

}

func StopAllVPNs() error {

	log.Infof(" ******************** STOPPING ALL VPNS ********************")

	file, err := os.Open(GetDataPath() + "nettica.json")

	if err != nil {
		log.Errorf("Error opening nettica.json file %v", err)
		return err
	}
	conf, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading nettica config file: %v", err)
		return err
	}

	var msg model.Message
	err = json.Unmarshal(conf, &msg)
	if err != nil {
		log.Errorf("Error reading message from config file")
		return err
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
			return err
		}

		vpn := msg.Config[i].VPNs[index]

		// Stop them all asynchronously or some might get missed
		go func(vpn model.VPN) {
			if vpn.Enable {
				log.Infof(" >>>>>>>>>> Stopping VPN %s <<<<<<<<<<", vpn.NetName)
				vpn.Enable = false
				err = UpdateVPN(&vpn)
				if err != nil {
					log.Errorf("Error disabling VPN %v", vpn.NetName)
				}
				err = StopWireguard(vpn.NetName)
				if err != nil {
					log.Errorf("Error stopping wireguard: %v", err)
				}
			}
		}(vpn)
	}

	// Marshal the message back to the file
	body, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("Error marshalling message: %v", err)
		return err
	}

	file, err = os.OpenFile(GetDataPath()+"nettica.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Errorf("Error opening nettica.json for write: %v", err)
		return nil
	}
	_, err = file.Write(body)
	file.Close()
	if err != nil {
		log.Infof("Error writing nettica.json file: %v", err)
		return nil
	}

	return nil
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

				// Get our local subnets
				subnets, err := GetLocalSubnets()
				if err != nil {
					log.Errorf("GetLocalSubnets, err = %v", err)
				}
				log.Errorf("Subnets: %v", subnets)

				// Iterate through this VPN's addresses and remove any subnets that match
				// What should be left is the subnets that are local to this device
				for k := 0; k < len(vpn.Current.Address); k++ {
					network, err := GetNetworkAddress(vpn.Current.Address[k])
					if err != nil {
						log.Errorf("GetNetworkAddress, err = %v", err)
						continue
					}
					for l := 0; l < len(subnets); l++ {
						if subnets[l].String() == network {
							log.Errorf("From Local: Removing subnet %s from %s", subnets[l].String(), vpn.Name)
							subnets = append(subnets[:l], subnets[l+1:]...)
						}
					}
				}

				// If any of the AllowedIPs contain a local subnet, remove that entry
				// This is to prevent routing loops and is very important
				for k := 0; k < len(msg.Config[i].VPNs); k++ {
					allowed := msg.Config[i].VPNs[k].Current.AllowedIPs
					for l := 0; l < len(allowed); l++ {
						inSubnet := false
						if !strings.Contains(allowed[l], "/") {
							continue
						}
						_, s, err := net.ParseCIDR(allowed[l])
						if err != nil {
							log.Errorf("net.ParseCIDR err = %v", err)
							continue
						}
						for _, subnet := range subnets {
							if subnet.Contains(s.IP) {
								log.Errorf("From Foreign: Removing subnet %s from %s", allowed[l], vpn.Name)
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

				running, err := IsWireguardRunning(msg.Config[i].NetName)
				if err != nil {
					log.Errorf("Error checking if wireguard is running: %v", err)
				}

				if !vpn.Enable {
					if running || err != nil {
						log.Infof("Net %s is disabled.  Stopped service...", msg.Config[i].NetName)
						StopWireguard(msg.Config[i].NetName)
					}
				} else {
					if !running || err != nil {
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

			}
		}

		StartDNS()

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
