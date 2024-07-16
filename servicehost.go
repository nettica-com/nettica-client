package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var netticaServiceHostAPIFmt = "%s/api/v1.0/service/%s/status"
var netticaServiceHostUpdateAPIFmt = "%s/api/v1.0/service/%s"

func StartServiceHost(s *Server, c chan bool) {
	var host string
	for s.Config.Device == nil {
		time.Sleep(1 * time.Second)
	}

	host = s.Config.Device.Server
	var client *http.Client
	var etag string

	err := StartContainers(s)
	if err != nil {
		log.Debugf("Error starting containers %v", err)
	}

	if strings.HasPrefix(host, "http:") {
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

	for {
		<-c
		if !cfg.loaded {
			err := loadConfig()
			if err != nil {
				log.Errorf("Failed to load config.")
			}
		}

		// Only make API call if ServiceGroup is set
		if s.Config.Device.ServiceGroup != "" && s.Config.Device.ServiceApiKey != "" {
			var reqURL string = fmt.Sprintf(netticaServiceHostAPIFmt, host, s.Config.Device.ServiceGroup)
			log.Debugf("  GET %s", reqURL)

			var buffer []byte
			req, err := http.NewRequest("GET", reqURL, bytes.NewBuffer(buffer))
			if err != nil {
				return
			}
			if req != nil {
				req.Header.Set("X-API-KEY", s.Config.Device.ServiceApiKey)
				req.Header.Set("User-Agent", "nettica-client/2.0")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("If-None-Match", etag)
			}
			resp, err := client.Do(req)
			if err == nil {

				if resp.StatusCode == 304 {
				} else if resp.StatusCode != 200 {
					log.Errorf("Response Error Code: %v, sleeping 10 seconds", resp.StatusCode)
					time.Sleep(10 * time.Second)
				} else {
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Errorf("error reading body %v", err)
					}
					log.Debugf("%s", string(body))
					etag = resp.Header.Get("ETag")
					UpdateServiceHostConfig(s, body)
				}
			} else {
				log.Errorf("ERROR: %v, sleeping 10 seconds", err)
				time.Sleep(10 * time.Second)
			}
			if resp != nil {
				resp.Body.Close()
			}
			if req != nil {
				req.Body.Close()
			}
		}

	}
}

func StartContainers(s *Server) error {

	path := s.Path
	path = strings.TrimSuffix(path, ".json")
	path = path + "-service-host.json"

	file, err := os.Open(path)

	if err != nil {
		log.Debugf("Error opening config file %v", err)
		return err
	}
	body, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Errorf("Error reading service host config file: %v", err)
		return err
	}
	var msg model.ServiceMessage
	err = json.Unmarshal(body, &msg)
	if err != nil {
		log.Errorf("Error unmarshalling service host config file: %v", err)
		return err
	}

	for _, service := range msg.Config {
		if service.ContainerId == "" {
			// Start the container
			id, err := StartService(service)
			if err != nil {
				log.Errorf("Error starting service %v", err)
			} else {
				service.ContainerId = id
				service.Status = "Running"
				UpdateNetticaServiceHost(s, service)
				log.Infof("Started service %s", service.ContainerId)
			}
		} else {
			// If the container isn't running (eg, reboot), restart it
			if !CheckService(service) {
				service.ContainerId = ""
				id, err := StartService(service)
				if err == nil {
					service.ContainerId = id
					service.Status = "Running"
					UpdateNetticaServiceHost(s, service)
					log.Infof("Restarted service %s", service.ContainerId)
				}
			}
		}
	}
	return nil
}

func UpdateNetticaServiceHost(s *Server, service model.Service) error {

	log.Infof("UPDATING SERVICE: %v", service)
	server := s.Config.Device.Server
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

	var reqURL string = fmt.Sprintf(netticaServiceHostUpdateAPIFmt, server, service.Id)
	log.Infof("  PATCH %s", reqURL)
	content, err := json.Marshal(service)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", reqURL, bytes.NewBuffer(content))
	if err != nil {
		return err
	}
	if req != nil {
		req.Header.Set("X-API-KEY", service.ApiKey)
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

// UpdateServiceHostConfig updates the config from the server
func UpdateServiceHostConfig(s *Server, body []byte) {

	// If the file doesn't exist create it for the first time
	if _, err := os.Stat(GetDataPath() + "my.nettica.com-service-host.json"); os.IsNotExist(err) {
		file, err := os.OpenFile(GetDataPath()+"my.nettica.com-service-host.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err == nil {
			file.Close()
		}
	}

	file, err := os.Open(GetDataPath() + "my.nettica.com-service-host.json")

	if err != nil {
		log.Errorf("Error opening service host config file %v", err)
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
		file, err := os.OpenFile(GetDataPath()+"my.nettica.com-service-host.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			log.Errorf("Error opening my.nettica.com-service-host.json for write: %v", err)
			return
		}
		_, err = file.Write(body)
		file.Close()
		if err != nil {
			log.Infof("Error writing my.nettica.com-service-host.json file: %v", err)
			return
		}
		var msg model.ServiceMessage
		err = json.Unmarshal(body, &msg)

		if err != nil {
			log.Errorf("Error reading message from server")
		}

		var oldmsg model.ServiceMessage
		err = json.Unmarshal(conf, &oldmsg)

		if err != nil {
			log.Errorf("Error reading message from disk")
		}

		log.Debugf("%v", msg)

		// Check and update the status of the container

		for _, service := range msg.Config {
			if service.ContainerId == "" {
				// Start the container
				id, err := StartService(service)
				if err != nil {
					log.Errorf("Error starting service %v", err)
					service.Status = "Error"
					UpdateNetticaServiceHost(s, service)
				} else {
					service.ContainerId = id
					service.Status = "Running"
					UpdateNetticaServiceHost(s, service)
				}
			} else {
				// If the container isn't running (eg, reboot), restart it
				if !CheckService(service) {
					service.ContainerId = ""
					id, err := StartService(service)
					if err == nil {
						service.ContainerId = id
						service.Status = "Running"
						UpdateNetticaServiceHost(s, service)
					} else {
						log.Errorf("Error restarting service %v", err)
						service.Status = "Error"
						UpdateNetticaServiceHost(s, service)
					}
				}
			}
		}

		// Remove any containers that are no longer in the config
		for _, oldservice := range oldmsg.Config {
			found := false
			for _, newservice := range msg.Config {
				if oldservice.ContainerId == newservice.ContainerId {
					found = true
				}
			}
			if !found {
				log.Infof("Removing container %s", oldservice.ContainerId)
				// Stop the container
				StopService(oldservice)
			}
		}
	}
}

// StartService starts a container
func StartService(service model.Service) (string, error) {

	var err error
	id, err := StartContainer(service)
	if err != nil {
		log.Errorf("Error starting container %v", err)
	} else {
		service.ContainerId = id
		log.Infof("Started container %s", service.ContainerId)
	}
	return id, err
}

// CheckService checks if a container is running
func CheckService(service model.Service) bool {

	if service.ContainerId != "" {
		running := CheckContainer(service)
		if !running {
			log.Infof("Container %s is not running", service.ContainerId)
			return false
		}
		return true
	}
	return false
}

// StopService stops a container
func StopService(service model.Service) {

	err := StopContainer(service)
	if err != nil {
		log.Errorf("Error stopping container %v", err)
	} else {
		log.Infof("Stopped container %s", service.ContainerId)
	}
}

// DoServiceWork catches any errors in the service and recovers from them, if possible
func DoServiceWork(s *Server) {
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

		c := make(chan bool)
		go StartServiceHost(s, c)

		curTs = calculateCurrentTimestamp()

		t := time.Unix(curTs, 0)
		log.Debugf("current timestamp = %v (%s)", curTs, t.UTC())

		for {
			time.Sleep(1000 * time.Millisecond)
			ts := time.Now()

			if ts.Unix() >= curTs {

				// call the channel to trigger the next poll
				c <- true

				curTs = calculateCurrentTimestamp()
				curTs += s.Config.Device.CheckInterval
			}

		}
	}()
}
