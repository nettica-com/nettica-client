package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func main() {

	path := "nettica.log"
	file, err := os.Open(GetDataPath() + path)
	if err != nil {

	} else {
		log.SetFormatter(&log.TextFormatter{})
		log.SetOutput(file)
		log.SetLevel(log.InfoLevel)
	}

	err = loadConfig()
	if err != nil && len(os.Args) < 2 {
		log.Error("Could not load config, will load when it is ready. err= ", err)
	}

	d, err := GetNetticaDevice()
	if err != nil {
		log.Errorf("Could not get device: %v", err)
	}
	if !CompareDevices(d, &device) {
		log.Infof("Device changed, saving config")
		device = *d
		err = saveConfig()
		if err != nil {
			log.Errorf("Could not save config: %v", err)
		}
		err = reloadConfig()
		if err != nil {
			log.Errorf("Could not reload config: %v", err)
		}
	}

	if !device.Enable {
		log.Info("Device is disabled, exiting")
		return
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
