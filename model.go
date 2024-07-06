package main

import "github.com/nettica-com/nettica-admin/model"

type Server struct {
	Name    string        `json:"name"`
	Path    string        `json:"path"`
	Config  model.Message `json:"config"`
	Body    []byte        `json:"body"`
	Running chan bool     `json:"-"`
	Worker  *Worker       `json:"-"`
}

type ClientWorker interface {
	StartServer()
	FailSafe()
	DiscoverDevice()
	CallNettica(etag string) ([]byte, error)
	GetNetticaDevice() (*model.Device, error)
	UpdateNetticaDevice(device *model.Device) error
	GetNetticaVPN(etag string) (string, error)
	UpdateVPN(vpn *model.VPN) error
	DeleteVPN(id string) error
	UpdateNetticaConfig(body []byte)
	ValidateMessage(msg *model.Message) error
	FindVPN(net string) (*model.VPN, *[]model.VPN, error)
	StopAllVPNs() error
	StartBackgroundRefreshService()
}
type Client struct {
	ClientWorker
	Context *Server `json:"context"`
}
