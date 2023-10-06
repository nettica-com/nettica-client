package main

import (
	"encoding/json"
	"flag"
	"net"
	"os"
	"runtime"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var device model.Device = model.Device{}

var cfg struct {
	sourceAddr *net.TCPAddr
	init       bool
	loaded     bool
	path       *string
}

type configError struct {
	message string
}

func (err *configError) Error() string {
	return err.message
}

func saveConfig() error {
	log.Info("Saving config")
	if cfg.path == nil {
		return nil
	}
	if device.CheckInterval == 0 {
		device.CheckInterval = 10
	}
	data, err := json.Marshal(device)
	if err != nil {
		return err
	}
	return os.WriteFile(GetDataPath()+*cfg.path, data, 0644)
}

func reloadConfig() error {
	log.Info("Reloading config")
	if cfg.path == nil {
		return nil
	}
	data, err := os.ReadFile(GetDataPath() + *cfg.path)
	if err != nil {
		return err
	}
	json.Unmarshal(data, &device)

	log.Infof("Server:   %s", device.Server)
	log.Infof("DeviceID: %s", device.Id)
	log.Infof("ApiKey:   %s", device.ApiKey)
	log.Infof("CheckInterval:   %d", device.CheckInterval)
	log.Infof("Quiet:    %t", device.Quiet)

	return nil
}

func loadConfig() error {

	if cfg.loaded {
		return nil
	}

	if !cfg.init {
		cfg.init = true

		// configure defaults
		device.Debug = false
		device.Quiet = false
		device.CheckInterval = 10
		device.SourceAddress = "0.0.0.0"
		device.OS = runtime.GOOS
		device.Architecture = runtime.GOARCH
		device.Version = Version
		device.Enable = true

		// load defaults from environment
		device.Server = os.Getenv("NETTICA_SERVER")
		device.Id = os.Getenv("NETTICA_DEVICE_ID")
		device.ApiKey = os.Getenv("NETTICA_API_KEY")
		device.ServiceGroup = os.Getenv("NETTICA_SERVICE_GROUP")
		device.ServiceApiKey = os.Getenv("NETTICA_SERVICE_API_KEY")

		if device.Server == "" {
			device.Server = "https://dev.nettica.com"
		}
		device.Enable = true

		// pick up command line arguments
		cfg.path = flag.String("C", "nettica.conf", "Path to configuration file")
		Server := flag.String("server", "", "Nettica server to connect to")
		DeviceID := flag.String("DeviceID", "", "Host ID to use")
		ServiceGroup := flag.String("servicegroup", "", "Service group to use")
		ServiceApiKey := flag.String("serviceapikey", "", "Service API key to use")

		ApiKey := flag.String("apikey", "", "API key to use")
		CheckInterval := flag.Int64("interval", 0, "Time interval between maps.  Default is 10 (seconds)")
		quiet := flag.Bool("quiet", false, "Do not output to stdout (only to syslog)")
		sourceStr := flag.String("source", "", "Source address for http client requests")
		flag.Parse()

		// Open the config file specified

		file, err := os.Open(GetDataPath() + *cfg.path)
		if err != nil && *Server == "" && *DeviceID == "" && *ApiKey == "" && device.Id == "" && device.ApiKey == "" {
			return err
		}

		// If we could open the config read it, otherwise go with cmd line args
		if err == nil {
			decoder := json.NewDecoder(file)
			err = decoder.Decode(&device)
			if err != nil {
				return err
			}
		}

		if *quiet {
			device.Quiet = *quiet
		}

		if *Server != "" {
			device.Server = *Server
		}
		if *DeviceID != "" {
			device.Id = *DeviceID
		}
		if *ApiKey != "" {
			device.ApiKey = *ApiKey
		}

		if *ServiceGroup != "" {
			device.ServiceGroup = *ServiceGroup
		}
		if *ServiceApiKey != "" {
			device.ServiceApiKey = *ServiceApiKey
		}

		if device.Server == "" {
			return &configError{"A nettica.conf file with a Server parameter is required"}
		}

		if *CheckInterval != 0 {
			device.CheckInterval = *CheckInterval
		}

		if *sourceStr != "" {
			device.SourceAddress = *sourceStr
		}

		cfg.sourceAddr, err = net.ResolveTCPAddr("tcp", device.SourceAddress+":0")
		if err != nil {
			return err
		}
		cfg.loaded = true
		log.Infof("Server:   %s", device.Server)
		log.Infof("DeviceID: %s", device.Id)
		log.Infof("ApiKey:   %s", device.ApiKey)
		log.Infof("Quiet:    %t", device.Quiet)

	} else {
		file, err := os.Open(GetDataPath() + *cfg.path)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(file)
		err = decoder.Decode(&device)
		if err != nil {
			return err
		}

		log.Infof("Server:   %s", device.Server)
		log.Infof("DeviceID: %s", device.Id)
		log.Infof("ApiKey:   %s", device.ApiKey)
		log.Infof("Quiet:    %t", device.Quiet)

		cfg.loaded = true
	}
	return nil
}
