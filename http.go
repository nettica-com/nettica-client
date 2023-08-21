package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

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
	netes := make(map[string]Metrics, 11)
	lines := strings.Split(body, ("\n"))
	for i := 0; i < len(lines); i++ {
		parts := strings.Fields(lines[i])
		if len(parts) < 3 {
			break
		}
		recv, _ := strconv.ParseInt(parts[1], 10, 0)
		send, _ := strconv.ParseInt(parts[2], 10, 0)

		net, found := netes[name]
		if !found {
			net = Metrics{Send: 0, Recv: 0}
		}
		net.Send += send
		net.Recv += recv
		netes[name] = net
	}
	result, err := json.Marshal(netes)
	return string(result), err
}

// statHandler will return the stats for the requested net
func statsHandler(w http.ResponseWriter, req *http.Request) {
	if !device.Quiet {
		log.Infof("statsHandler")
	}
	// /stats/
	parts := strings.Split(req.URL.Path, "/")
	net := parts[2]
	if !device.Quiet {
		log.Infof("GetStats(%s)", net)
	}

	// GetStats will execute "wg show net transfer" and return the output
	body, err := GetStats(net)
	if err != nil {
		log.Error(err)
	}

	// which we then make into a json structure and return it
	stats, err := MakeStats(net, body)
	if err != nil {
		log.Error(err)
	}
	if !device.Quiet {
		log.Infof("Stats: %s", stats)
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

func stopServiceHandler(w http.ResponseWriter, req *http.Request) {
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
	net := parts[2]
	log.Infof("StopWireguard(%s)", net)

	switch req.Method {
	case "DELETE":
		log.Infof("Method: %s", req.Method)
		err := StopWireguard(net)
		if err != nil {
			log.Error(err)
		}
		io.WriteString(w, "")

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

	switch req.Method {
	case "GET":
		log.Infof("Method: %s", req.Method)
		Server := req.URL.Query().Get("server")
		DeviceID := req.URL.Query().Get("id")
		ApiKey := req.URL.Query().Get("apiKey")
		if Server != "" && DeviceID != "" && ApiKey != "" {
			device.Server = Server
			device.Id = DeviceID
			device.ApiKey = ApiKey

			CheckInterval, _ := strconv.ParseInt(req.URL.Query().Get("checkInterval"), 10, 0)
			if CheckInterval != 0 {
				device.CheckInterval = CheckInterval
			}

			appData := req.URL.Query().Get("appdata")
			if appData != "" {
				device.AppData = appData
			}

			accountid := req.URL.Query().Get("accountid")
			if accountid != "" {
				device.AccountID = accountid
			}

			name := req.URL.Query().Get("name")
			if name != "" {
				device.Name = name
			}

			AuthDomain := req.URL.Query().Get("authDomain")
			if AuthDomain != "" {
				device.AuthDomain = AuthDomain
			}

			clientID := req.URL.Query().Get("clientid")
			if clientID != "" {
				device.ClientID = clientID
			}

			apiID := req.URL.Query().Get("apiid")
			if apiID != "" {
				device.ApiID = apiID
			}

			os := req.URL.Query().Get("os")
			if os != "" {
				device.OS = os
			}

			arch := req.URL.Query().Get("arch")
			if arch != "" {
				device.Architecture = arch
			}

			saveConfig()
			UpdateNetticaDevice(device)

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
	http.HandleFunc("/service/", stopServiceHandler)
	http.HandleFunc("/config/", configHandler)

	log.Infof("Starting web server on %s", ":53280")

	err := http.ListenAndServe(":53280", nil)
	if err != nil {
		log.Error(err)
	}

}
