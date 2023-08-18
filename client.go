package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nettica-com/nettica-admin/model"
	util "github.com/nettica-com/nettica-admin/util"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var netticaHostAPIFmt = "%s/api/v1.0/device/%s/status"
var netticaHostUpdateAPIFmt = "%s/api/v1.0/device/%s"

// Start the channel that iterates the nettica update function
func StartChannel(c chan []byte) {

	log.Infof("StartChannel Nettica Host %s", config.Host)
	etag := ""
	var err error

	for {
		content := <-c
		if content == nil {
			break
		}

		etag, err = GetNetticaConfig(etag)
		if err != nil {
			log.Errorf("Error getting nettica config: %v", err)
		}
	}
}

func CallNettica(etag *string) ([]byte, error) {

	vpn := config.Host
	var client *http.Client

	if strings.HasPrefix(vpn, "http:") {
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
				LocalAddr: config.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaHostAPIFmt, vpn, config.DeviceID)
	if !config.Quiet {
		log.Infof("  GET %s", reqURL)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", config.ApiKey)
		req.Header.Set("User-Agent", "nettica-client/2.0")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-None-Match", *etag)
	}
	resp, err := client.Do(req)
	if err == nil {
		if resp.StatusCode == 304 {
			buffer, err := ioutil.ReadFile(GetDataPath() + "nettica.json")
			if err == nil {
				return buffer, nil
			}

		} else if resp.StatusCode == 401 {
			return nil, fmt.Errorf("Unauthorized")
		} else if resp.StatusCode != 200 {
			log.Errorf("Response Error Code: %v", resp.StatusCode)
			return nil, fmt.Errorf("Response Error Code: %v", resp.StatusCode)
		} else {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading body %v", err)
			}
			log.Debugf("%s", string(body))

			etag2 := resp.Header.Get("ETag")

			if *etag != etag2 {
				log.Infof("etag = %s  etag2 = %s", *etag, etag2)
				*etag = etag2
			} else {
				log.Infof("etag %s is equal", etag2)
			}
			if resp != nil {
				resp.Body.Close()
			}

			return body, nil
		}
	} else {
		log.Errorf("ERROR: %v, continuing", err)
	}

	return nil, err

}

func GetNetticaConfig(etag string) (string, error) {

	if !config.loaded {
		err := loadConfig()
		if err != nil {
			log.Errorf("Failed to load config.")
		}
	}

	body, err := CallNettica(&etag)
	if err != nil {
		if err.Error() == "Unauthorized" {
			log.Errorf("Unauthorized - reload config")
			// Read the config and find another API key
			// pick up any changes from the agent or manually editing the config file.
			reloadConfig()

		} else {
			log.Error(err)
		}
	} else {
		UpdateNetticaConfig(body)
		return etag, nil
	}

	return "", err
}

func UpdateVPN(vpn model.VPN) error {

	log.Infof("UPDATING VPN: %v", vpn)
	server := config.Host
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
				LocalAddr: config.sourceAddr,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
		}

	}

	var reqURL string = fmt.Sprintf(netticaHostUpdateAPIFmt, server, vpn.Id)
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
		req.Header.Set("X-API-KEY", config.ApiKey)
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
	conf, err := ioutil.ReadAll(file)
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
			log.Errorf("Error reading message from disk")
		}

		log.Debugf("%v", msg)

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
			log.Errorf("GetLocalSubnets, err = ", err)
		}

		// first, delete any netes that are no longer in the conf
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
				StopWireguard(oldconf.Config[i].NetName)
				os.Remove(GetDataPath() + oldconf.Config[i].NetName + ".conf")

				for _, vpn := range oldconf.Config[i].VPNs {
					if vpn.DeviceID == config.DeviceID {
						KeyDelete(vpn.Current.PublicKey)
						KeySave()
					}
				}
			}
		}

		// handle any other changes
		for i := 0; i < len(msg.Config); i++ {
			index := -1
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == config.DeviceID {
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
					UpdateVPN(vpn2)

				} else {
					vpn.Current.PrivateKey = key
				}

				text, err := DumpWireguardConfig(&vpn, &(msg.Config[i].VPNs))
				if err != nil {
					log.Errorf("error on template: %s", err)
				}

				// Check the current file and if it's an exact match, do not bounce the service
				path := GetWireguardPath()

				force := false
				var bits []byte

				file, err := os.Open(path + msg.Config[i].NetName + ".conf")
				if err != nil {
					log.Errorf("Error opening %s for read: %v", msg.Config[i].NetName, err)
					force = true
				} else {
					bits, err = ioutil.ReadAll(file)
					file.Close()
					if err != nil {
						log.Errorf("Error reading nettica config file: %v", err)
						force = true
					}
				}

				if !force && bytes.Equal(bits, text) {
					log.Infof("*** SKIPPING %s *** No changes!", msg.Config[i].NetName)
				} else {
					err = StopWireguard(msg.Config[i].NetName)
					if err != nil {
						log.Errorf("Error stopping wireguard: %v", err)
					}

					err = util.WriteFile(path+msg.Config[i].NetName+".conf", text)
					if err != nil {
						log.Errorf("Error writing file %s : %s", path+msg.Config[i].NetName+".conf", err)
					}

					if !vpn.Enable {
						// Host was disabled when we stopped wireguard above
						log.Infof("Net %s is disabled.  Stopped service if running.", msg.Config[i].NetName)
						// Stopping the service doesn't seem very reliable, stop it again
						if err = StopWireguard(msg.Config[i].NetName); err != nil {
							log.Errorf("Error stopping wireguard: %v", err)
						}

					} else {
						err = StartWireguard(msg.Config[i].NetName)
						if err == nil {
							log.Infof("Started %s", msg.Config[i].NetName)
							log.Infof("%s Config: %v", msg.Config[i].NetName, msg.Config[i])
						}
					}
				}

			}
		}
	}

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

func StartBackgroundRefreshService() {

	for {

		file, err := os.Open(GetDataPath() + "nettica.json")
		if err != nil {
			log.Errorf("Error opening nettica.json for read: %v", err)
			return
		}
		bytes, err := ioutil.ReadAll(file)
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

		// Get our local subnets, called here to avoid duplication
		subnets, err := GetLocalSubnets()
		if err != nil {
			log.Errorf("GetLocalSubnets, err = ", err)
		}

		for i := 0; i < len(msg.Config); i++ {
			index := -1
			for j := 0; j < len(msg.Config[i].VPNs); j++ {
				if msg.Config[i].VPNs[j].DeviceID == config.DeviceID {
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
					UpdateVPN(vpn2)

				} else {
					vpn.Current.PrivateKey = key
				}

				text, err := DumpWireguardConfig(&vpn, &(msg.Config[i].VPNs))
				if err != nil {
					log.Errorf("error on template: %s", err)
				}
				path := GetWireguardPath()
				err = util.WriteFile(path+msg.Config[i].NetName+".conf", text)
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
			time.Sleep(100 * time.Millisecond)
			ts := time.Now()

			if ts.Unix() >= curTs {

				b := []byte("")

				c <- b

				curTs = calculateCurrentTimestamp()
				curTs += config.CheckInterval
			}

		}
	}()
}

func calculateCurrentTimestamp() int64 {

	return time.Now().Unix()

}
