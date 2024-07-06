package main

import (
	"net"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// var device model.Device = model.Device{}

// ServiceHost can only be set using an environment variable
var ServiceHost = false

var cfg struct {
	sourceAddr *net.TCPAddr
	init       bool
	loaded     bool
	path       *string
	quiet      bool
	debug      bool
	Server     string
	DeviceID   string
	ApiKey     string
	UpdateKeys bool
}

type configError struct {
	message string
}

func (err *configError) Error() string {
	return err.message
}

func loadConfig() error {

	if cfg.loaded {
		return nil
	}

	if !cfg.init {
		cfg.init = true

		_, ServiceHost = os.LookupEnv("NETTICA_SERVICE_HOST")

		// configure defaults
		cfg.debug = false
		cfg.quiet = true
		cfg.UpdateKeys = true

		// load defaults from environment
		cfg.Server = os.Getenv("NETTICA_SERVER")
		cfg.DeviceID = os.Getenv("NETTICA_DEVICE_ID")
		cfg.ApiKey = os.Getenv("NETTICA_API_KEY")

		value, qpresent := os.LookupEnv("NETTICA_QUIET")
		if qpresent {
			if value == "" || strings.ToLower(value) == "true" || value == "1" {
				cfg.quiet = true
			} else {
				cfg.quiet = false
			}
		}

		value, dpresent := os.LookupEnv("NETTICA_DEBUG")
		if dpresent {
			if value == "" || strings.ToLower(value) == "true" || value == "1" {
				cfg.debug = true
			} else {
				cfg.debug = false
			}
		}

		if strings.ToLower(os.Getenv("NETTICA_UPDATE_KEYS")) == "false" {
			cfg.UpdateKeys = false
		}

		if cfg.Server == "" {
			cfg.Server = "https://my.nettica.com"
		}

		var err error
		cfg.sourceAddr, err = net.ResolveTCPAddr("tcp", "0.0.0.0:0")
		if err != nil {
			return err
		}
		cfg.loaded = true
		log.Infof("Server:    %s", cfg.Server)
		log.Infof("DeviceID:  %s", cfg.DeviceID)
		log.Infof("ApiKey:    %s...", cfg.ApiKey[0:len(cfg.ApiKey)-len(cfg.ApiKey)/2])
		log.Infof("Quiet:     %t", cfg.quiet)

		cfg.loaded = true
	}
	return nil
}
