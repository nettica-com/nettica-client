package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func ReadFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	b, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Output json structures for the stats and key generation

type Metrics struct {
	Send int64
	Recv int64
}

type Key struct {
	Public string
}

func MakeStats(name string, body string) (string, error) {
	nets := make(map[string]Metrics, 11)
	lines := strings.Split(body, ("\n"))
	for i := 0; i < len(lines); i++ {
		parts := strings.Fields(lines[i])
		if len(parts) < 3 {
			break
		}
		recv, _ := strconv.ParseInt(parts[1], 10, 0)
		send, _ := strconv.ParseInt(parts[2], 10, 0)

		net, found := nets[name]
		if !found {
			net = Metrics{Send: 0, Recv: 0}
		}
		net.Send += send
		net.Recv += recv
		nets[name] = net
	}
	result, err := json.Marshal(nets)
	return string(result), err
}

// statHandler will return the stats for the requested net
func statsHandler(w http.ResponseWriter, req *http.Request) {
	// /stats/
	parts := strings.Split(req.URL.Path, "/")
	net := Sanitize(parts[2])

	// GetStats will execute "wg show net transfer" and return the output
	body, _ := GetStats(net)

	// which we then make into a json structure and return it
	stats, err := MakeStats(net, body)
	if err != nil {
		log.Error(err)
	}

	w.Header().Add("Access-Control-Allow-Origin", "*")
	io.WriteString(w, stats)
}

// keyHandler will generate a new keypair and insert it into the keystore.
// It will then return the public key.  This allows the agent to create a new
// host without compromising the private key.
func keyHandler(w http.ResponseWriter, req *http.Request) {
	log.Infof("keyHandler")
	// /keys/

	// add the headers here to pass preflight checks
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "*")

	switch req.Method {
	case "GET":
		log.Infof("Method: %s", req.Method)
		key := Key{}
		wg, _ := wgtypes.GeneratePrivateKey()
		key.Public = wg.PublicKey().String()
		KeyAdd(key.Public, wg.String())
		KeySave()
		json.NewEncoder(w).Encode(key)

	default:
		log.Infof("Method: %s", req.Method)
		io.WriteString(w, "")
		log.Infof("Unknown method: %s", req.Method)
	}

}

func ServiceHandler(w http.ResponseWriter, req *http.Request) {
	// add the headers here to pass preflight checks
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "*")
	w.Header().Add("Access-Control-Allow-Headers", "*")

	// extract the net name from the url
	parts := strings.Split(req.URL.Path, "/")
	if len(parts) < 3 {
		log.Errorf("Invalid url: %s", req.URL.Path)
		io.WriteString(w, "")
		return
	}
	net := parts[2]
	net = Sanitize(net)

	switch req.Method {
	case "DELETE":

		var s *Server
		var vpn *model.VPN
		var err error
		found := false
		log.Infof("Method: %s", req.Method)

		for _, s = range Servers {
			vpn, _, _ = s.Worker.FindVPN(net)
			if vpn != nil {
				found = true
				break
			}
		}
		if !found {
			log.Errorf("VPN %s not found", net)
			// return an error
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var errvpn error
		if vpn != nil {
			vpn.Enable = false
			errvpn = s.Worker.UpdateVPN(vpn)
			if errvpn != nil {
				log.Error(errvpn)
			} else {
				log.Infof("Update successful.  VPN %s is disabled", net)
			}
			if vpn.Current.EnableDns {
				DropCache()
				UpdateDNS()
			}
		}

		log.Infof("StopWireguard(%s)", net)
		err = StopWireguard(net)
		if err != nil {
			log.Error(err)
		}

		if vpn != nil && errvpn != nil {
			vpn.Enable = false
			err = s.Worker.UpdateVPN(vpn)
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Update successful.  VPN %s is disabled", net)
			}
		}

		msg := fmt.Sprintf("Stopped network %s", net)
		NotifyInfo(msg)
		log.Info(msg)

		io.WriteString(w, "")

	case "PATCH":
		log.Infof("StartWireguard(%s)", net)
		log.Infof("Method: %s", req.Method)
		var s *Server
		var vpn *model.VPN
		var vpns *[]model.VPN
		var err error
		found := false

		for _, s = range Servers {
			vpn, vpns, _ = s.Worker.FindVPN(net)
			if vpn != nil {
				found = true
				break
			}
		}
		if !found {
			log.Errorf("VPN %s not found", net)
			// return an error
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if vpn != nil {
			found := false
			for _, v := range *vpns {
				for _, a := range v.Current.AllowedIPs {
					if a == "0.0.0.0/0" || a == "::/0" {
						found = true
						break
					}
				}
				if found {
					break
				}
			}

			if found {
				log.Infof("Stopping all other VPNs")
				msg := fmt.Sprintf("Connecting to %s.  Stopping all other VPNs", net)
				NotifyInfo(msg)
				for _, ss := range Servers {
					err = ss.Worker.StopAllVPNs()
					if err != nil {
						log.Error(err)
					}
				}
			}

			vpn.Enable = true
			err = s.Worker.UpdateVPN(vpn)
			if err != nil {
				log.Error(err)
			}
		}

		err = StartWireguard(net)
		if err != nil {
			log.Error(err)
		}
		msg := fmt.Sprintf("Started network %s", net)
		NotifyInfo(msg)
		log.Info(msg)

		io.WriteString(w, "")

	default:
		io.WriteString(w, "")
		log.Infof("Unknown method: %s", req.Method)
	}
}

func vpnHandler(w http.ResponseWriter, req *http.Request) {
	// add the headers here to pass preflight checks
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "*")

	// extract the net name from the url
	parts := strings.Split(req.URL.Path, "/")
	if len(parts) < 3 {
		log.Errorf("Invalid url: %s", req.URL.Path)
		io.WriteString(w, "")
		return
	}
	vpnid := parts[2]

	switch req.Method {
	case "DELETE":
		log.Infof("Method: %s %s", req.Method, req.URL.Path)

		var s *Server
		for _, s = range Servers {
			vpn, _, err := s.Worker.FindVPNById(vpnid)
			if err == nil {
				if vpn != nil {
					if vpn.Id == vpnid {
						log.Infof("Found VPN %s", vpnid)
						err := s.Worker.DeleteVPN(vpnid)
						if err != nil {
							log.Error(err)
						}
						io.WriteString(w, "")
						return
					}
				}
			}
		}
		log.Errorf("VPN %s not found", vpnid)
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "")

	default:
		io.WriteString(w, "")
		log.Infof("Unknown method: %s", req.Method)
	}
}

func deviceHandler(w http.ResponseWriter, req *http.Request) {
	// add the headers here to pass preflight checks
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "*")

	// extract the net name from the url
	parts := strings.Split(req.URL.Path, "/")
	if len(parts) < 3 {
		log.Errorf("Invalid url: %s", req.URL.Path)
		io.WriteString(w, "")
		return
	}
	deviceid := parts[2]

	switch req.Method {
	case "DELETE":
		log.Infof("Method: %s %s", req.Method, req.URL.Path)

		var s *Server
		for _, s = range Servers {
			if s.Config.Device.Id == deviceid {
				log.Infof("Found Device %s", deviceid)

				// first delete all the VPNs associated with this device
				vpn_configs := s.Config.Config
				for _, vpn_config := range vpn_configs {
					for _, vpn := range vpn_config.VPNs {
						if vpn.DeviceID == deviceid {
							log.Infof("Found VPN %s", vpn.Id)
							err := s.Worker.DeleteVPN(vpn.Id)
							if err != nil {
								log.Error(err)
							}
						}
					}
				}

				// then delete the device from the server

				err := s.Worker.DeleteDevice(deviceid)
				if err != nil {
					log.Error(err)
				}

				// finally delete the server
				s.Running <- false
				delete(Servers, s.Path)
				err = os.Remove(s.Path)
				if err != nil {
					log.Error(err)
				}

				io.WriteString(w, "")
				return
			}
		}
		log.Errorf("Device %s not found", deviceid)
		// return a 404 error
		w.WriteHeader(http.StatusNotFound)
		return

	default:
		io.WriteString(w, "")
		log.Infof("Unknown method: %s", req.Method)
	}
}

func configHandler(w http.ResponseWriter, req *http.Request) {
	// decode the GET request parameters and save them to the config

	// add the headers here to pass preflight checks
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "*")

	device := model.Device{}
	device.Version = Version
	device.OS = runtime.GOOS
	device.Architecture = runtime.GOARCH
	device.Version = Version
	device.Enable = true
	device.CheckInterval = 10
	device.Name, _ = os.Hostname()
	device.SourceAddress = "0.0.0.0"
	device.InstanceID = InstanceID

	switch req.Method {
	case "GET":
		log.Infof("Method: %s configHandler", req.Method)
		Srvr := req.URL.Query().Get("server")
		if Srvr == "undefined" {
			Srvr = ""
		}
		DeviceID := req.URL.Query().Get("id")
		if DeviceID == "undefined" {
			DeviceID = ""
		}
		ApiKey := req.URL.Query().Get("apiKey")
		if ApiKey == "undefined" {
			ApiKey = ""
		}
		EZCode := req.URL.Query().Get("ezcode")
		if EZCode == "undefined" {
			EZCode = ""
		}
		if strings.HasPrefix(EZCode, "ez-") || (Srvr != "") {
			device.Server = Srvr
			device.Id = DeviceID
			device.ApiKey = ApiKey
			device.EZCode = EZCode

			CheckInterval, _ := strconv.ParseInt(req.URL.Query().Get("checkInterval"), 10, 0)
			if CheckInterval != 0 {
				device.CheckInterval = CheckInterval
			}

			accountid := req.URL.Query().Get("accountid")
			if accountid != "" && accountid != "undefined" {
				device.AccountID = accountid
			}

			name := req.URL.Query().Get("name")
			if name != "" {
				device.Name = name
			}

			os := req.URL.Query().Get("os")
			if os != "" {
				device.OS = os
			}

			arch := req.URL.Query().Get("arch")
			if arch != "" {
				device.Architecture = arch
			}

			instanceid := req.URL.Query().Get("instanceid")
			if instanceid != "" && instanceid != "undefined" {
				device.InstanceID = instanceid
			}

			device.Updated = time.Now()

			found := false

			for _, s := range Servers {
				if s.Config.Device.Server == Srvr {
					found = true

					if device.EZCode != "" {
						s.Config.Device.EZCode = device.EZCode
					}

					if device.ApiKey != "" {
						s.Config.Device.ApiKey = device.ApiKey
					}

					if device.Id != "" {
						s.Config.Device.Id = device.Id
					}

					s.Config.Device.OS = runtime.GOOS
					s.Config.Device.Architecture = runtime.GOARCH
					s.Config.Device.Version = Version
					s.Config.Device.Enable = true
					s.Config.Device.CheckInterval = 10
					s.Config.Device.Version = Version

					SaveServer(s)
					s.Worker.UpdateNetticaDevice(device)

					break

				}
			}

			if !found {
				msg := model.Message{}
				msg.Device = &device

				name := device.Server
				if strings.HasPrefix(strings.ToLower(name), "https://") {
					name = strings.ToLower(name[8:])
				} else if strings.HasPrefix(strings.ToLower(name), "http://") {
					name = strings.ToLower(name[7:])
				}

				server := NewServer(name, msg)
				SaveServer(server)
				go func(s *Server) {

					log.Infof("Server: %v", s)

					w := Worker{Context: s}
					s.Worker = &w

					go w.StartServer()
					go w.StartBackgroundRefreshService()

					curTs := calculateCurrentTimestamp()

					t := time.Unix(curTs, 0)
					log.Debugf("current timestamp = %v (%s)", curTs, t.UTC())

					for {
						time.Sleep(1000 * time.Millisecond)
						ts := time.Now()

						if ts.Unix() >= curTs {

							w.Context.Running <- true

							curTs = calculateCurrentTimestamp()
							curTs += w.Context.Config.Device.CheckInterval
						}

					}

				}(server)

			}

		} else {
			log.Error("Invalid config parameters")
		}
	}
	data, err := json.Marshal(device)
	if err != nil {
		return
	}
	// write the config back to the caller
	io.WriteString(w, string(data))

}

func startHTTPd() {
	http.HandleFunc("/stats/", statsHandler)
	http.HandleFunc("/keys/", keyHandler)
	http.HandleFunc("/service/", ServiceHandler)
	http.HandleFunc("/vpn/", vpnHandler)
	http.HandleFunc("/device/", deviceHandler)
	http.HandleFunc("/config/", configHandler)

	log.Infof("Starting web server on %s", "127.0.0.1:53280")

	err := http.ListenAndServe("127.0.0.1:53280", nil)
	if err != nil {
		log.Error(err)
	}

}
