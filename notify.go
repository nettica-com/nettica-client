package main

import (
	"encoding/json"
	"fmt"
	"net"

	model "github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var (
	lastDNS  string
	lastInfo string
)

// Notify sends a message to 127.0.0.1:25264, the Nettica Agent app

func NotifyDNS(name string) {
	var note model.AgentNotification
	note.Type = "dns"
	note.Text = name

	if name == lastDNS {
		log.Debugf("Skipping duplicate DNS notification: %s", name)
		return
	}

	lastDNS = name

	bytes, err := json.Marshal(note)
	if err != nil {
		log.Errorf("Error marshalling notification: %v", err)
		return
	}

	err = Notify(bytes)
	if err != nil {
		log.Errorf("Error sending notification: %v", err)
	}
}

func NotifyInfo(message string) {
	var note model.AgentNotification
	note.Type = "info"
	note.Text = message

	if message == lastInfo {
		log.Debugf("Skipping duplicate info notification: %s", message)
		return
	}

	lastInfo = message

	bytes, err := json.Marshal(note)
	if err != nil {
		log.Errorf("Error marshalling notification: %v", err)
		return
	}

	err = Notify(bytes)
	if err != nil {
		log.Errorf("Error sending notification: %v", err)
	}
}

func Notify(message []byte) error {
	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:25264")
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}

	defer conn.Close()

	//conn.WriteMsgUDP(message, nil, raddr)
	fmt.Fprint(conn, string(message))

	log.Debugf("Notification: %v", string(message))

	return nil
}
