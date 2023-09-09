package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

func GetWireguardPath() string {
	return "/etc/wireguard/"
}

func GetDataPath() string {
	return "/etc/nettica/"
}

// Return the platform
func Platform() string {
	return "Linux"
}

func GetStats(net string) (string, error) {
	args := []string{"show", net, "transfer"}

	out, err := exec.Command("wg", args...).Output()
	if err != nil {
		log.Errorf("Error getting statistics: %v (%s)", err, string(out))
		return "", err
	}

	return string(out), nil
}

func InstallWireguard(netName string) error {
	return StartWireguard(netName)
}

func RemoveWireguard(netName string) error {
	return StopWireguard(netName)
}

func StartWireguard(netName string) error {

	args := []string{"wg-quick", "up", netName}

	cmd := exec.Command("/bin/bash", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error starting WireGuard: %v (%s)", err, out.String())
		return err
	}

	return err

}

func StopWireguard(netName string) error {

	args := []string{"wg-quick", "down", netName}

	cmd := exec.Command("/bin/bash", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error stopping WireGuard: %v (%s)", err, out.String())
	}

	// remove the file if it exists
	path := GetWireguardPath() + netName + ".conf"
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}

	return err

}

// docker run -e NETTICA_HOST_ID=715d2d3d-2eb2-4f06-be90-4e8d679360a5 -e NETTICA_API_KEY=example -p 40000:40000 nettica-client

func StartContainer(service model.Service) (string, error) {

	port := fmt.Sprintf("%d", service.ServicePort)

	var args = []string{"run", "--rm", "-d", "--cap-add", "NET_ADMIN", "--cap-add", "SYS_MODULE", "--sysctl", "net.ipv4.conf.all.src_valid_mark=1", "-e", "NETTICA_SERVER=" + service.Device.Server, "-e", "NETTICA_DEVICE_ID=" + service.Device.Id, "-e", "NETTICA_API_KEY=" + service.Device.ApiKey, "-p", port + ":" + port + "/udp", "nettica-client"}
	cmd := exec.Command("docker", args...)
	log.Infof("Starting container: %v", cmd)

	var outerr bytes.Buffer
	var outstd bytes.Buffer
	cmd.Stderr = &outerr
	cmd.Stdout = &outstd

	err := cmd.Run()
	if err != nil {
		log.Errorf("Error starting container: %v (%s)", err, outerr.String())
		return "", err
	}

	service.Status = "Running"
	if outstd.String() != "" && outstd.String() != "\n" {
		service.ContainerId = outstd.String()
		service.ContainerId = strings.TrimSuffix(service.ContainerId, "\n")
	}

	return service.ContainerId, nil
}

// check the status of the container
func CheckContainer(service model.Service) bool {
	// docker container ls -qf id=3f268613a949
	var args = []string{"container", "ls", "-qf", "id=" + service.ContainerId}

	cmd := exec.Command("docker", args...)

	var outerr bytes.Buffer
	var outstd bytes.Buffer
	cmd.Stderr = &outerr
	cmd.Stdout = &outstd

	err := cmd.Run()
	if err != nil {
		log.Errorf("Error checking container: %v (%s)", err, outerr.String())
		return false
	}

	if outstd.String() == service.ContainerId {
		return true
	}
	if outstd.String() != "" {
		return true
	}

	return false
}

// docker stop service.ContainerId
func StopContainer(service model.Service) error {

	var args = []string{"kill", service.ContainerId}
	cmd := exec.Command("docker", args...)

	var outerr bytes.Buffer
	var outstd bytes.Buffer
	cmd.Stderr = &outerr
	cmd.Stdout = &outstd

	err := cmd.Run()
	if err != nil {
		log.Errorf("Error killing container: %v (%s)", err, outerr.String())
		return err
	}

	service.ContainerId = ""
	service.Status = "Stopped"

	return nil
}

func InService() (bool, error) {
	return true, nil
}

func RunService(svcName string) {

	DoWork()
	DoServiceWork()

	log.Info("setting up signal handlers")
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
	os.Exit(0)

}

func ServiceManager(svcName string, cmd string) {

}
