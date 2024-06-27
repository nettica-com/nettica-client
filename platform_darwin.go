package main

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/miekg/dns"
	"github.com/nettica-com/nettica-admin/model"
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

	net = Sanitize(net)

	// find the utun interface from the network name

	file, err := os.ReadFile("/var/run/wireguard/" + net + ".name")
	if err != nil {
		return "", err
	}
	utun := string(file)
	utun = utun[:len(utun)-1]

	args := []string{"show", utun, "transfer"}

	out, err := exec.Command("./wg", args...).Output()
	if err != nil {
		log.Debugf("Error getting statistics: %v (%s)", err, string(out))
		return "nodata 0 0", err
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

	netName = Sanitize(netName)

	go func() {

		path := GetWireguardPath() + netName + ".conf"

		args := []string{"-f", path}
		cmd := exec.Command("./wireguard-go", args...)
		var out bytes.Buffer
		cmd.Stderr = &out
		err := cmd.Start()
		if err != nil {
			log.Errorf("Error starting WireGuard:%s %v (%s)", netName, err, out.String())
			return
		}
	}()

	go func() {
		args := []string{"wg-quick", "up", netName}
		cmd := exec.Command("./bash", args...)
		var out bytes.Buffer
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
			log.Errorf("Error starting WireGuard:%s %v (%s)", netName, err, out.String())
			return
		}

		FlushDNS()

	}()

	return nil

}
func StopWireguard(netName string) error {

	netName = Sanitize(netName)

	args := []string{"wg-quick", "down", netName}

	cmd := exec.Command("./bash", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error stopping WireGuard:%s %v (%s)", netName, err, out.String())
	}
	// remove the file if it exists
	path := GetWireguardPath() + netName + ".conf"
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}

	FlushDNS()

	return err

}

func IsWireguardRunning(name string) (bool, error) {

	name = Sanitize(name)

	cmd := exec.Command("wg", "show", name)
	err := cmd.Run()
	if err != nil {
		return false, nil
	}

	return true, nil
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

var AppleServer *DNS_SERVER

func InitializeDNS() error {

	AppleServer = &DNS_SERVER{}

	go func() {
		AppleServer.udp = &dns.Server{Addr: "127.0.0.1:53", Net: "udp", TsigSecret: nil, ReusePort: true}
		if err := AppleServer.udp.ListenAndServe(); err != nil {
			log.Warnf("UpdateDNS: Failed to setup the UDP DNS server on %s: %s", "127.0.0.1:53", err.Error())
		}
	}()

	go func() {
		AppleServer.tcp = &dns.Server{Addr: "127.0.0.1:53", Net: "tcp", TsigSecret: nil, ReusePort: true}
		if err := AppleServer.udp.ListenAndServe(); err != nil {
			log.Warnf("UpdateDNS: Failed to setup the TCP DNS server on %s: %s", "127.0.0.1:53", err.Error())
		}
	}()

	return nil
}

func LaunchDNS(addr string) (*DNS_SERVER, error) {
	return AppleServer, nil
}

func FlushDNS() {
	log.Info("==================== Flushing DNS ====================")

	go func() {

		cmd := exec.Command("dscacheutil", "-flushcache")
		err := cmd.Run()
		if err != nil {
			log.Errorf("Error flushing DNS: %v", err)
		}
		cmd = exec.Command("killall", "mDNSResponder")
		err = cmd.Run()
		if err != nil {
			log.Errorf("Error flushing DNS: %v", err)
		}
	}()
}
