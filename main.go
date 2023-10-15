package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

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

	if device.Id == "" {
		go DiscoverDevice(&device)
	}

	d, err := GetNetticaDevice()
	if err != nil {
		log.Errorf("Could not get device: %v", err)
	}
	merged := false

	if d != nil {

		if !CompareDevices(d, &device) {
			log.Infof("Device changed, saving config")
			MergeDevices(d, &device)
			merged = true
			err = saveConfig()
			if err != nil {
				log.Errorf("Could not save config: %v", err)
			}
			err = reloadConfig()
			if err != nil {
				log.Errorf("Could not reload config: %v", err)
			}
		}
	}

	if !device.Enable {
		log.Info("Device is disabled, exiting")
		return
	}

	if merged {
		err = UpdateNetticaDevice(device)
		if err != nil {
			log.Errorf("Could not update device: %v", err)
		}
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
		DoServiceWork()

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
