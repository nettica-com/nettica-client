package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var Version = "development"

func main() {

	path := "nettica.log"
	file, err := os.OpenFile(GetDataPath()+path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Info("Create a file named nettica.log in the nettica directory if you want to capture logs to a file")
	} else {
		log.SetFormatter(&log.TextFormatter{})
		log.SetOutput(file)
		log.SetLevel(log.InfoLevel)
	}

	log.Infof("Nettica Client %s", Version)

	// Ensure the data directory exists
	err = os.MkdirAll(GetDataPath(), 0755)
	if err != nil {
		log.Errorf("Could not create data directory: %v", err)
	}

	err = loadConfig()
	if err != nil && len(os.Args) < 2 {
		log.Error("Could not load config, will load when it is ready. err= ", err)
	}

	log.SetLevel(log.InfoLevel)

	if cfg.quiet {
		log.SetLevel(log.ErrorLevel)
	}

	if cfg.debug {
		log.SetLevel(log.DebugLevel)
	}

	log.SetLevel(log.InfoLevel)

	// Migrate if needed
	Migrate()

	// If the config is set by environment variables, ignore
	// the config file
	if cfg.Server != "" && cfg.DeviceID != "" && cfg.ApiKey != "" {
		log.Info("Using environment variables for configuration")
		msg := model.Message{}
		msg.Device = &model.Device{}
		msg.Device.Version = Version
		msg.Device.Server = cfg.Server
		msg.Device.Id = cfg.DeviceID
		msg.Device.ApiKey = cfg.ApiKey
		msg.Device.UpdateKeys = cfg.UpdateKeys
		msg.Device.Enable = true
		msg.Device.CheckInterval = 10

		if cfg.debug {
			msg.Device.Logging = "debug"
		} else if cfg.quiet {
			msg.Device.Logging = "error"
		} else {
			msg.Device.Logging = "info"
		}

		// Not getting the return value server because it has
		// been added to the Servers list and will be started shortly
		_ = NewServer(msg.Device.Server, msg)
	}

	KeyInitialize()
	KeyLoad()

	const svcName = "nettica"

	inService, _ := InService()
	if inService {
		RunService(svcName)
		return
	}

	if len(os.Args) > 1 {
		cmd := strings.ToLower(os.Args[1])

		ServiceManager(svcName, cmd)

		return
	} else {
		log.Infof("Nettica Control Plane Started")

		DoWork()

		sigs := make(chan os.Signal, 1)
		done := make(chan bool, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			sig := <-sigs
			log.Errorf("%v", sig)
			done <- true
		}()

		<-done

		log.Info("Exiting")
	}

}
