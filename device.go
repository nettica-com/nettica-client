package main

import (
	"net"
	"os"
	"strings"
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

func loadConfig() error {

	if cfg.loaded {
		return nil
	}

	if !cfg.init {
		cfg.init = true

		_, ServiceHost = os.LookupEnv("NETTICA_SERVICE_HOST")

		// configure defaults
		cfg.debug = false
		cfg.quiet = false
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
	}
	return nil
}
