package main

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

// Notify sends a message to 127.0.0.1:25265, the Nettica Agent app
func Notify(message string) error {
	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:25265")
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.WriteMsgUDP([]byte(message), nil, raddr)

	fmt.Fprint(conn, message)

	log.Infof("Notification: %v", message)

	return nil
}
