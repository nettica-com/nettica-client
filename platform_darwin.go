package main

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func GetWireguardPath() string {
	return "/usr/local/etc/wireguard/"
}

func GetDataPath() string {
	return "/usr/local/etc/nettica/"
}

// Return the platform
func Platform() string {
	return "MacOS"
}

func GetStats(net string) (string, error) {
	args := []string{"wg", "show", net, "transfer"}

	out, err := exec.Command("/usr/local/bin/bash", args...).Output()
	if err != nil {
		log.Errorf("Error getting statistics: %v (%s)", err, string(out))
		return "", err
	}

	return string(out), nil
}

func Startireguard(netName string) error {

	args := []string{"wg-quick", "up", netName}

	cmd := exec.Command("/usr/local/bin/bash", args...)
	cmd.Stderr = &out
	go func() {
		err := cmd.Run()
	}()

	if err != nil {
		log.Errorf("Error reloading WireGuard: %v (%s)", err, out.String())
		return err
	}

	return err

}
func StopWireguard(netName string) error {

	args := []string{"wg-quick", "down", netName}

	cmd := exec.Command("/usr/local/bin/bash", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error reloading WireGuard: %v (%s)", err, out.String())
	}
	// remove the file if it exists
	path := GetWireguardPath() + netName + ".conf"
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}

	return err

}

func StartContainer(service model.Service) (string, error) {
	return "", nil
}

func CheckContainer(service model.Service) bool {
	return true
}

func StopContainer(service model.Service) error {
	return nil
}

func InService() (bool, error) {
	return true, nil
}

func RunService(svcName string) {
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

func ServiceManager(svcName string, cmd string) {
}
