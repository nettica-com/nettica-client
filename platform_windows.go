package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/miekg/dns"
	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

// GetWireguardPath finds wireguard location for the given platform
func GetWireguardPath() string {

	path := GetDataPath() + "Wireguard\\"
	return path
}

func GetDataPath() string {
	return "C:\\ProgramData\\Nettica\\"
}

// Return the platform
func Platform() string {
	return "Windows"
}

func GetStats(net string) (string, error) {
	args := []string{"show", net, "transfer"}
	out, err := exec.Command("wg.exe", args...).Output()
	if err != nil {
		log.Errorf("Error getting stats: %v (%s)", err, string(out))
		return "", err
	}
	return string(out), nil
}

func InstallWireguard(netName string) error {

	time.Sleep(1 * time.Second)

	args := []string{"/installtunnelservice", GetWireguardPath() + netName + ".conf"}

	var out bytes.Buffer
	cmd := exec.Command("wireguard.exe", args...)
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error installing WireGuard tunnel: %v (%s)", err, out.String())
		return err
	}

	// open the service and set it to manual
	//	args = []string{"config", "WireGuardTunnel$" + netName, "start= demand"}
	//	cmd = exec.Command("sc.exe", args...)
	//	cmd.Stderr = &out
	//	err = cmd.Run()
	//	if err != nil {
	//		log.Errorf("Error setting WireGuard service to manual: %v (%s)", err, out.String())
	//		return err
	//	}

	// open the service and set it to manual
	m, err := mgr.Connect()
	if err != nil {
		log.Errorf("Error connecting to service manager: %v", err)
		return err
	}
	defer m.Disconnect()

	service, err := m.OpenService("WireGuardTunnel$" + netName)
	if err != nil {
		log.Errorf("Error opening service: %v", err)
		return err
	}
	defer service.Close()

	config, err := service.Config()
	if err != nil {
		log.Errorf("Error getting service config: %v", err)
		return err
	}

	config.StartType = mgr.StartManual
	err = service.UpdateConfig(config)
	if err != nil {
		log.Errorf("Error updating service config: %v", err)
		return err
	}

	return nil

}

func RemoveWireguard(netName string) error {

	args := []string{"/uninstalltunnelservice", netName}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wireguard.exe", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Start()
	if err != nil {
		log.Errorf("Error removing WireGuard tunnel: %v (%s)", err, out.String())
	}
	log.Info(out.String())

	err = cmd.Wait()
	if err != nil {
		log.Errorf("Error removing WireGuard tunnel: %v (%s)", err, out.String())
	}

	// remove the file if it exists
	path := GetWireguardPath() + netName + ".conf"
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}

	return err

}

// StartWireguard restarts the wireguard tunnel on the given platform
func StartWireguard(netName string) error {

	// Start the existing wireguard service
	// example: net stop WireGuardTunnel$london

	args := []string{"start", "WireGuardTunnel$" + netName}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "net.exe", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Start()
	if err != nil {
		log.Errorf("Error starting WireGuard: %v (%s)", err, out.String())
	}
	log.Info(out.String())

	err = cmd.Wait()
	if err != nil {
		log.Errorf("Error starting WireGuard: %v (%s)", err, out.String())
	}

	return err

}

// StopWireguard stops the wireguard tunnel on the given platform
func StopWireguard(netName string) error {

	// Stop the existing wireguard service
	// example: net stop WireGuardTunnel$london

	args := []string{"stop", "WireGuardTunnel$" + netName}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "net.exe", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Start()
	if err != nil {
		log.Errorf("Error stopping WireGuard: %v (%s)", err, out.String())
	}
	log.Info(out.String())

	err = cmd.Wait()
	if err != nil {
		log.Errorf("Error stopping WireGuard: %v (%s)", err, out.String())
	}

	return err

}

func IsWireguardRunning(netName string) (bool, error) {
	// Check if the wireguard service is running for the network
	// using the service control manager

	m, err := mgr.Connect()
	if err != nil {
		log.Errorf("Error connecting to service manager: %v", err)
		return false, err
	}
	defer m.Disconnect()

	service, err := m.OpenService("WireGuardTunnel$" + netName)
	if err != nil {
		log.Errorf("Error opening service: %v", err)
		return false, err
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		log.Errorf("Error querying service: %v", err)
		return false, err
	}

	if status.State == svc.Running {
		return true, nil
	}

	return false, nil

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

// Windows Main functions

func InService() (bool, error) {
	inService, err := svc.IsWindowsService()

	if err != nil {
		log.Fatalf("failed to determine if we are running in service: %v", err)
	}
	return inService, err
}

func RunService(svcName string) {
	runService(svcName, false)
}

func ServiceManager(svcName string, cmd string) {
	var err error
	switch cmd {
	case "debug":
		runService(svcName, true)
		return
	case "install":
		err = installService(svcName, "Nettica Agent")
	case "remove":
		err = removeService(svcName)
	case "makenet":
		err = makeMesh(os.Args[2])
	case "removenet":
		err = removeMesh(os.Args[2])
	case "start":
		err = startService(svcName)
	case "stop":
		err = controlService(svcName, svc.Stop, svc.Stopped)
	case "pause":
		err = controlService(svcName, svc.Pause, svc.Paused)
	case "continue":
		err = controlService(svcName, svc.Continue, svc.Running)
	default:
		usage(fmt.Sprintf("invalid command %s", cmd))
	}
	if err != nil {
		log.Infof("failed to %s %s: %v", cmd, svcName, nil)
		os.Exit(0)
	}

}

func InitializeDNS() error {

	return nil
}

func LaunchDNS(address string) (*dns.Server, error) {

	server := &dns.Server{Addr: address, Net: "udp", TsigSecret: nil, ReusePort: true}
	log.Infof("Starting DNS Server on %s", address)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Errorf("Failed to setup the DNS server on %s: %s\n", address, err.Error())
		}
	}()

	return server, nil
}
